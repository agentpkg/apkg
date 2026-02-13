package serve

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/agentpkg/agentpkg/pkg/config"
	"github.com/agentpkg/agentpkg/pkg/container"
	"github.com/agentpkg/agentpkg/pkg/store"
	"github.com/pelletier/go-toml/v2"
)

const (
	// DefaultPort is the default listen port for the proxy.
	DefaultPort = 19513
	// MCPServerHeader is the HTTP header used to route requests by name.
	MCPServerHeader = "X-MCP-Server"
	// MCPServerDigestHeader disambiguates when multiple installs use the
	// same server name with different images.
	MCPServerDigestHeader = "X-MCP-Server-Digest"
)

// containerKey uniquely identifies a managed container by name + digest.
type containerKey struct {
	name   string
	digest string
}

// Server is the apkg serve HTTP proxy. It lazily starts containers on first
// request and reverse-proxies traffic to them.
type Server struct {
	Port        int
	IdleTimeout time.Duration
	Engine      *container.Engine
	Containers  map[containerKey]*managedContainer
}

// NewServerFromStore creates a Server by scanning the store's oci/ directory
// for installed container MCP servers. Each subdirectory at
// oci/<name>/<digest>/mcp.toml describes a container server.
func NewServerFromStore(st store.Store, port int, engine *container.Engine) (*Server, error) {
	containers, err := discoverContainers(st)
	if err != nil {
		return nil, fmt.Errorf("discovering containers: %w", err)
	}

	return &Server{
		Port:        port,
		IdleTimeout: DefaultIdleTimeout,
		Engine:      engine,
		Containers:  containers,
	}, nil
}

// discoverContainers walks the store's oci/ directory and reads mcp.toml
// files to build the set of available containers.
// Store layout: oci/<name>/<digest>/mcp.toml
func discoverContainers(st store.Store) (map[containerKey]*managedContainer, error) {
	containers := make(map[containerKey]*managedContainer)

	ociDir := st.Path("oci")
	nameEntries, err := os.ReadDir(ociDir)
	if err != nil {
		if os.IsNotExist(err) {
			return containers, nil
		}
		return nil, fmt.Errorf("reading oci directory: %w", err)
	}

	for _, nameEntry := range nameEntries {
		if !nameEntry.IsDir() {
			continue
		}
		name := nameEntry.Name()

		digestEntries, err := os.ReadDir(filepath.Join(ociDir, name))
		if err != nil {
			continue
		}

		for _, digestEntry := range digestEntries {
			if !digestEntry.IsDir() {
				continue
			}
			digest := digestEntry.Name()

			mcpPath := filepath.Join(ociDir, name, digest, "mcp.toml")
			data, err := os.ReadFile(mcpPath)
			if err != nil {
				continue
			}

			var ms config.MCPSource
			if err := toml.Unmarshal(data, &ms); err != nil {
				log.Printf("warning: skipping invalid mcp.toml at %s: %v", mcpPath, err)
				continue
			}

			if ms.ContainerMCPConfig == nil || ms.Image == "" {
				continue
			}

			containerPort := 8080
			if ms.Port != nil {
				containerPort = *ms.Port
			}

			mc := &managedContainer{
				name:          name,
				image:         ms.Image,
				containerPort: containerPort,
				volumes:       ms.Volumes,
				network:       ms.Network,
			}
			if ms.LocalMCPConfig != nil {
				mc.env = ms.Env
				mc.args = ms.Args
			}

			key := containerKey{name: name, digest: digest}
			containers[key] = mc
		}
	}

	return containers, nil
}

// ListenAndServe starts the proxy and blocks until a shutdown signal is
// received. It returns nil on clean shutdown.
func (s *Server) ListenAndServe(ctx context.Context) error {
	if len(s.Containers) == 0 {
		return fmt.Errorf("no containerized HTTP MCP servers found in store")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start the idle reaper in the background.
	go startIdleReaper(ctx, s.Engine, s.Containers, s.IdleTimeout)

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.proxyHandler)

	addr := fmt.Sprintf("127.0.0.1:%d", s.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Graceful shutdown on signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		log.Printf("apkg serve listening on %s", addr)
		for key := range s.Containers {
			digest := key.digest
			if len(digest) > 12 {
				digest = digest[:12]
			}
			log.Printf("  %s [%s] → %s (lazy start)", key.name, digest, s.Containers[key].image)
		}
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err == http.ErrServerClosed {
			break
		}
		cancel()
		return err
	case sig := <-sigCh:
		log.Printf("received %v, shutting down", sig)
	case <-ctx.Done():
		log.Printf("context cancelled, shutting down")
	}

	// Cancel the reaper.
	cancel()

	// Shut down the HTTP server.
	if err := srv.Shutdown(context.Background()); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	// Stop all containers.
	all := make([]*managedContainer, 0, len(s.Containers))
	for _, mc := range s.Containers {
		all = append(all, mc)
	}
	if err := stopAllContainers(context.Background(), s.Engine, all); err != nil {
		log.Printf("error stopping containers: %v", err)
	}

	return nil
}

// proxyHandler routes requests based on the X-MCP-Server and
// X-MCP-Server-Digest headers, lazily starting containers on first request
// and reusing the cached reverse proxy for subsequent requests.
func (s *Server) proxyHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received request: %s %s", r.Method, r.URL.Path)
	log.Printf("  Headers: %v", r.Header)
	serverName := r.Header.Get(MCPServerHeader)
	if serverName == "" {
		http.Error(w, fmt.Sprintf("missing %s header", MCPServerHeader), http.StatusBadRequest)
		return
	}

	digest := r.Header.Get(MCPServerDigestHeader)
	key := containerKey{name: serverName, digest: digest}

	mc, ok := s.Containers[key]
	if !ok {
		http.Error(w, fmt.Sprintf("unknown MCP server %q (digest %q)", serverName, digest), http.StatusNotFound)
		return
	}

	if err := mc.ensureRunning(r.Context(), s.Engine); err != nil {
		log.Printf("failed to start container for %q: %v", serverName, err)
		http.Error(w, fmt.Sprintf("failed to start MCP server %q: %v", serverName, err),
			http.StatusServiceUnavailable)
		return
	}

	mc.touch()

	mc.mu.Lock()
	proxy := mc.proxy
	mc.mu.Unlock()

	if proxy == nil {
		http.Error(w, fmt.Sprintf("MCP server %q is not ready", serverName), http.StatusServiceUnavailable)
		return
	}

	proxy.ServeHTTP(w, r)
}
