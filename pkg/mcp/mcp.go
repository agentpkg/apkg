package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agentpkg/agentpkg/pkg/config"
	"github.com/agentpkg/agentpkg/pkg/container"
	"github.com/pelletier/go-toml/v2"
)

// detectContainerEngine is the function used to find the container runtime.
// It defaults to container.DetectEngine and can be overridden in tests.
var detectContainerEngine = container.DetectEngine

const (
	mcpConfigFile  = "mcp.toml"
	transportStdio = "stdio"
	transportHTTP  = "http"

	// serveProxyURL is the default URL for the apkg serve proxy that
	// manages containerized MCP servers. Must match serve.DefaultPort.
	serveProxyURL = "http://localhost:19513"
	// serveRouteHeader is the HTTP header used by apkg serve to route
	// requests to the correct container. Must match serve.MCPServerHeader.
	serveRouteHeader = "X-MCP-Server"
	// serveRouteDigestHeader disambiguates when multiple projects install
	// the same server name with different images. Must match serve.MCPServerDigestHeader.
	serveRouteDigestHeader = "X-MCP-Server-Digest"
)

type MCPServer interface {
	Name() string
	Validate() error

	// used by projectors to write agent-specific config
	// do not necessarily return all the underlying apkg config
	Transport() string
	Command() string
	Args() []string
	URL() string
	Headers() map[string]string
	Env() map[string]string
}

func Load(dir string) (MCPServer, error) {
	configFile := filepath.Join(dir, mcpConfigFile)

	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read %q: %w", configFile, err)
	}

	cfg := &config.MCPSource{}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %q: %w", configFile, err)
	}

	if cfg.ManagedStdioMCPConfig != nil {
		var binPath string
		var err error

		switch {
		case strings.HasPrefix(cfg.Package, "npm:"):
			binPath, err = resolveNPMBin(dir, cfg.Package)
		case strings.HasPrefix(cfg.Package, "uv:"):
			binPath, err = resolveUVBin(dir, cfg.Package)
		case strings.HasPrefix(cfg.Package, "go:"):
			binPath, err = resolveGoBin(dir, cfg.Package)
		default:
			return nil, fmt.Errorf("unsupported managed package prefix in %q", cfg.Package)
		}
		if err != nil {
			return nil, fmt.Errorf("resolving binary for %q: %w", cfg.Package, err)
		}

		server := &localStdioMcpServer{
			name:    cfg.Name,
			command: binPath,
		}
		if cfg.LocalMCPConfig != nil {
			server.args = cfg.Args
			server.env = cfg.Env
		}
		return server, nil
	}

	if cfg.UnmanagedStdioMCPConfig != nil {
		server := &localStdioMcpServer{
			name:    cfg.Name,
			command: cfg.Command,
		}
		if cfg.LocalMCPConfig != nil {
			server.args = cfg.Args
			server.env = cfg.Env
		}
		return server, nil
	}

	if cfg.ExternalHttpMCPConfig != nil {
		server := &httpMCPServer{
			name:      cfg.Name,
			url:       cfg.URL,
			transport: cfg.Transport,
		}
		if cfg.HttpMCPConfig != nil {
			server.headers = cfg.Headers
		}
		return server, nil
	}

	if cfg.ContainerMCPConfig != nil && cfg.Image != "" {
		if cfg.Transport == transportStdio {
			return loadContainerStdio(cfg)
		}

		headers := map[string]string{
			serveRouteHeader:       cfg.Name,
			serveRouteDigestHeader: cfg.Digest,
		}
		// Merge user-configured headers (e.g. Authorization) — these get
		// forwarded to the container while the routing headers are stripped
		// by the apkg serve proxy.
		if cfg.HttpMCPConfig != nil {
			for k, v := range cfg.Headers {
				headers[k] = v
			}
		}
		serverURL := serveProxyURL + cfg.Path
		return &httpMCPServer{
			name:      cfg.Name,
			url:       serverURL,
			transport: transportHTTP,
			headers:   headers,
		}, nil
	}

	return nil, fmt.Errorf("unsupported MCP server configuration")
}

// loadContainerStdio builds a localStdioMcpServer that runs the container
// image via the detected container engine (docker/podman) with stdin attached.
func loadContainerStdio(cfg *config.MCPSource) (MCPServer, error) {
	engine, err := detectContainerEngine()
	if err != nil {
		return nil, fmt.Errorf("detecting container engine for stdio container: %w", err)
	}

	imageRef := cfg.Image
	if cfg.Digest != "" {
		imageRef = cfg.Image + "@sha256:" + cfg.Digest
	}

	runArgs := []string{"run", "--rm", "-i"}

	if cfg.Network != "" {
		runArgs = append(runArgs, "--network", cfg.Network)
	}

	for _, vol := range cfg.Volumes {
		runArgs = append(runArgs, "-v", vol)
	}

	if cfg.LocalMCPConfig != nil {
		// Sort env keys for deterministic arg ordering.
		keys := make([]string, 0, len(cfg.Env))
		for k := range cfg.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			runArgs = append(runArgs, "-e", k+"="+cfg.Env[k])
		}
	}

	runArgs = append(runArgs, imageRef)

	if cfg.LocalMCPConfig != nil {
		runArgs = append(runArgs, cfg.Args...)
	}

	return &localStdioMcpServer{
		name:    cfg.Name,
		command: engine.Path,
		args:    runArgs,
	}, nil
}

// resolveNPMBin finds the executable binary for an npm package installed at dir.
// It reads the package's package.json bin field and applies the same resolution
// logic as npx: if there's a single entry use it, otherwise match the unscoped
// package name.
func resolveNPMBin(dir string, pkg string) (string, error) {
	pkgName := strings.TrimPrefix(pkg, "npm:")
	if idx := strings.LastIndex(pkgName, "@"); idx > 0 {
		pkgName = pkgName[:idx]
	}

	pkgJSON := filepath.Join(dir, "node_modules", pkgName, "package.json")
	data, err := os.ReadFile(pkgJSON)
	if err != nil {
		return "", fmt.Errorf("reading package.json: %w", err)
	}

	var meta struct {
		Bin json.RawMessage `json:"bin"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return "", fmt.Errorf("parsing package.json: %w", err)
	}

	binDir := filepath.Join(dir, "node_modules", ".bin")

	// bin can be a string (single binary, name = unscoped package name)
	var single string
	if err := json.Unmarshal(meta.Bin, &single); err == nil {
		unscopedName := pkgName
		if i := strings.LastIndex(unscopedName, "/"); i >= 0 {
			unscopedName = unscopedName[i+1:]
		}
		return filepath.Join(binDir, unscopedName), nil
	}

	// bin can be a map of name -> path
	var bins map[string]string
	if err := json.Unmarshal(meta.Bin, &bins); err != nil {
		return "", fmt.Errorf("unexpected bin field format in package.json")
	}

	if len(bins) == 1 {
		for name := range bins {
			return filepath.Join(binDir, name), nil
		}
	}

	// multiple entries: match the unscoped package name
	unscopedName := pkgName
	if i := strings.LastIndex(unscopedName, "/"); i >= 0 {
		unscopedName = unscopedName[i+1:]
	}
	if _, ok := bins[unscopedName]; ok {
		return filepath.Join(binDir, unscopedName), nil
	}

	return "", fmt.Errorf("package %q has multiple bin entries and none match the package name %q", pkg, unscopedName)
}

type localStdioMcpServer struct {
	name    string
	command string
	args    []string
	env     map[string]string
}

func (s *localStdioMcpServer) Name() string {
	return s.name
}

func (s *localStdioMcpServer) Validate() error {
	if s.command == "" {
		return fmt.Errorf("unable to create command to run managed mcp server")
	}

	return nil
}

func (s *localStdioMcpServer) Transport() string {
	return transportStdio
}

func (s *localStdioMcpServer) Command() string {
	return s.command
}

func (s *localStdioMcpServer) Args() []string {
	return s.args
}

func (s *localStdioMcpServer) URL() string {
	return ""
}

func (s *localStdioMcpServer) Headers() map[string]string {
	return nil
}

func (s *localStdioMcpServer) Env() map[string]string {
	return s.env
}

type httpMCPServer struct {
	name      string
	url       string
	transport string
	headers   map[string]string
}

func (s *httpMCPServer) Name() string      { return s.name }
func (s *httpMCPServer) Transport() string  { return s.transport }
func (s *httpMCPServer) Command() string    { return "" }
func (s *httpMCPServer) Args() []string     { return nil }
func (s *httpMCPServer) URL() string        { return s.url }
func (s *httpMCPServer) Headers() map[string]string { return s.headers }
func (s *httpMCPServer) Env() map[string]string     { return nil }

func (s *httpMCPServer) Validate() error {
	if s.url == "" {
		return fmt.Errorf("external HTTP MCP server has no URL configured")
	}
	return nil
}

// resolveUVBin finds the executable binary for a uv package installed at dir.
// It looks for the binary at .venv/bin/<package-name> inside the install directory.
func resolveUVBin(dir string, pkg string) (string, error) {
	pkgName := strings.TrimPrefix(pkg, "uv:")
	if idx := strings.Index(pkgName, "=="); idx >= 0 {
		pkgName = pkgName[:idx]
	}

	binPath := filepath.Join(dir, ".venv", "bin", pkgName)
	if _, err := os.Stat(binPath); err != nil {
		return "", fmt.Errorf("binary not found at %s: %w", binPath, err)
	}

	return binPath, nil
}

// resolveGoBin finds the executable binary for a go module installed at dir.
// It looks for the binary at bin/<last-segment-of-module-path> inside the
// install directory. For example, github.com/go-delve/mcp-dap-server produces
// bin/mcp-dap-server.
func resolveGoBin(dir string, pkg string) (string, error) {
	modPath := strings.TrimPrefix(pkg, "go:")
	if idx := strings.LastIndex(modPath, "@"); idx > 0 {
		modPath = modPath[:idx]
	}

	// The binary name is the last segment of the module path.
	binName := modPath
	if i := strings.LastIndex(modPath, "/"); i >= 0 {
		binName = modPath[i+1:]
	}

	binPath := filepath.Join(dir, "bin", binName)
	if _, err := os.Stat(binPath); err != nil {
		return "", fmt.Errorf("binary not found at %s: %w", binPath, err)
	}

	return binPath, nil
}
