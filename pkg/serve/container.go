package serve

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/agentpkg/agentpkg/pkg/container"
)

// containerStatus tracks the lifecycle state of a managed container.
type containerStatus int

const (
	statusStopped containerStatus = iota
	statusStarting
	statusRunning
)

const (
	containerPrefix    = "apkg-"
	healthTimeout      = 30 * time.Second
	healthPollInterval = 250 * time.Millisecond

	// DefaultIdleTimeout is how long a container can be idle before it is
	// automatically stopped. Containers are restarted on the next request.
	DefaultIdleTimeout = 15 * time.Minute

	// idleCheckInterval is how often the reaper checks for idle containers.
	idleCheckInterval = 1 * time.Minute
)

// managedContainer represents a single container-based MCP server managed by
// the serve proxy.
type managedContainer struct {
	name          string // server name (matches X-MCP-Server header)
	image         string
	containerPort int
	hostPort      int
	env           map[string]string
	args          []string
	volumes       []string
	network       string

	mu       sync.Mutex
	status   containerStatus
	proxy    *httputil.ReverseProxy // cached proxy, created after container starts
	lastUsed time.Time              // updated on each proxied request
}

// containerName returns the docker/podman container name used for this server.
func (mc *managedContainer) containerName() string {
	return containerPrefix + mc.name
}

// touch updates the last-used timestamp under the lock.
func (mc *managedContainer) touch() {
	mc.mu.Lock()
	mc.lastUsed = time.Now()
	mc.mu.Unlock()
}

// ensureRunning is idempotent: if the container is already running it
// returns immediately (no liveness check — errors are caught by the
// proxy error handler). If the container is stopped it pulls the image,
// starts it, waits for TCP readiness, and builds a cached reverse proxy.
//
// Concurrent callers block on the mutex — only the first one starts the
// container.
func (mc *managedContainer) ensureRunning(ctx context.Context, engine *container.Engine) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.status == statusRunning {
		return nil
	}

	mc.status = statusStarting

	if err := engine.Pull(ctx, mc.image); err != nil {
		mc.status = statusStopped
		return err
	}

	// Clean up any stale container with the same name.
	_ = engine.Stop(ctx, mc.containerName())

	if mc.network == "host" {
		// With host networking the container shares the host network
		// stack, so no port mapping is needed — the container's port
		// is reachable directly.
		mc.hostPort = mc.containerPort
	} else {
		port, err := freePort()
		if err != nil {
			mc.status = statusStopped
			return fmt.Errorf("finding free port: %w", err)
		}
		mc.hostPort = port
	}

	opts := &container.RunOpts{
		Env:     mc.env,
		Args:    mc.args,
		Volumes: mc.volumes,
		Network: mc.network,
	}
	if _, err := engine.Run(ctx, mc.containerName(), mc.image, mc.hostPort, mc.containerPort, opts); err != nil {
		mc.status = statusStopped
		return err
	}

	if err := waitForTCP(ctx, mc.hostPort); err != nil {
		_ = engine.Stop(ctx, mc.containerName())
		mc.status = statusStopped
		return fmt.Errorf("container %q did not become ready: %w", mc.name, err)
	}

	mc.proxy = mc.buildProxy(engine)
	mc.lastUsed = time.Now()
	mc.status = statusRunning
	return nil
}

// stopLocked stops the container and resets state. Must be called with mc.mu held.
func (mc *managedContainer) stopLocked(ctx context.Context, engine *container.Engine) error {
	var err error
	if mc.status != statusStopped {
		err = engine.Stop(ctx, mc.containerName())
	}
	mc.status = statusStopped
	mc.proxy = nil
	return err
}

// stopIfIdle stops the container if it has been idle longer than timeout.
// Returns true if the container was stopped.
func (mc *managedContainer) stopIfIdle(ctx context.Context, engine *container.Engine, timeout time.Duration) bool {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.status != statusRunning {
		return false
	}
	if time.Since(mc.lastUsed) <= timeout {
		return false
	}

	log.Printf("stopping idle container %q (idle for %v)", mc.name, timeout)
	if err := mc.stopLocked(ctx, engine); err != nil {
		log.Printf("error stopping idle container %q: %v", mc.name, err)
	}
	return true
}

// buildProxy creates an httputil.ReverseProxy targeting the container's
// host port. The proxy strips apkg routing headers before forwarding,
// supports SSE streaming, and marks the container as stopped on
// connection errors so the next request triggers a restart.
func (mc *managedContainer) buildProxy(engine *container.Engine) *httputil.ReverseProxy {
	target := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("127.0.0.1:%d", mc.hostPort),
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
			// Strip apkg routing headers; user headers pass through.
			req.Header.Del(MCPServerHeader)
			req.Header.Del(MCPServerDigestHeader)
		},
		// FlushInterval -1 enables streaming/SSE support.
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("proxy error for %q: %v; marking container as stopped", mc.name, err)
			mc.mu.Lock()
			mc.status = statusStopped
			mc.proxy = nil
			mc.mu.Unlock()
			http.Error(w, fmt.Sprintf("MCP server %q is unavailable: %v", mc.name, err),
				http.StatusBadGateway)
		},
	}
	return proxy
}

// freePort asks the OS for an available TCP port by binding to :0.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}

// waitForTCP polls until a TCP connection to 127.0.0.1:port succeeds or the
// context/timeout expires.
func waitForTCP(ctx context.Context, port int) error {
	deadline := time.Now().Add(healthTimeout)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for TCP on %s", addr)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}

		time.Sleep(healthPollInterval)
	}
}

// stopAllContainers stops and removes all managed containers.
func stopAllContainers(ctx context.Context, engine *container.Engine, containers []*managedContainer) error {
	var errs []error
	for _, mc := range containers {
		mc.mu.Lock()
		if err := mc.stopLocked(ctx, engine); err != nil {
			errs = append(errs, fmt.Errorf("stopping %q: %w", mc.name, err))
		}
		mc.mu.Unlock()
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors stopping containers: %v", errs)
	}
	return nil
}

// startIdleReaper launches a background goroutine that periodically stops
// containers that haven't received a request within the idle timeout.
// It returns when ctx is cancelled.
func startIdleReaper(ctx context.Context, engine *container.Engine, containers map[containerKey]*managedContainer, idleTimeout time.Duration) {
	ticker := time.NewTicker(idleCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, mc := range containers {
				mc.stopIfIdle(ctx, engine, idleTimeout)
			}
		}
	}
}
