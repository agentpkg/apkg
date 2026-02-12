package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentpkg/agentpkg/pkg/config"
	"github.com/pelletier/go-toml/v2"
)

const (
	mcpConfigFile  = "mcp.toml"
	transportStdio = "stdio"
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
		binPath, err := resolveNPMBin(dir, cfg.Package)
		if err != nil {
			return nil, fmt.Errorf("resolving binary for %q: %w", cfg.Package, err)
		}

		server := &managedLocalMCPServer{
			name:    cfg.Name,
			command: binPath,
		}
		if cfg.StdioMCPConfig != nil {
			server.args = cfg.Args
		}
		if cfg.LocalMCPConfig != nil {
			server.env = cfg.Env
		}
		return server, nil
	}

	return nil, fmt.Errorf("unsupported MCP server configuration")
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

type managedLocalMCPServer struct {
	name    string
	command string
	args    []string
	env     map[string]string
}

func (s *managedLocalMCPServer) Name() string {
	return s.name
}

func (s *managedLocalMCPServer) Validate() error {
	if s.command == "" {
		return fmt.Errorf("unable to create command to run managed mcp server")
	}

	return nil
}

func (s *managedLocalMCPServer) Transport() string {
	return transportStdio
}

func (s *managedLocalMCPServer) Command() string {
	return s.command
}

func (s *managedLocalMCPServer) Args() []string {
	return s.args
}

func (s *managedLocalMCPServer) URL() string {
	return ""
}

func (s *managedLocalMCPServer) Headers() map[string]string {
	return nil
}

func (s *managedLocalMCPServer) Env() map[string]string {
	return s.env
}
