package serve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentpkg/agentpkg/pkg/container"
	"github.com/agentpkg/agentpkg/pkg/store"
)

// seedOCIStore writes an mcp.toml into the store at oci/<name>/<digest>/mcp.toml.
func seedOCIStore(t *testing.T, st store.Store, name, digest, tomlContent string) {
	t.Helper()
	dir := st.Path("oci", name, digest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating oci dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mcp.toml"), []byte(tomlContent), 0o644); err != nil {
		t.Fatalf("writing mcp.toml: %v", err)
	}
}

func TestNewServerFromStore(t *testing.T) {
	tests := map[string]struct {
		seed      func(t *testing.T, st store.Store)
		wantCount int
		wantKeys  []containerKey
		wantPorts map[containerKey]int
	}{
		"single container": {
			seed: func(t *testing.T, st store.Store) {
				seedOCIStore(t, st, "postgres", "abc123", `
transport = "http"
name = "postgres"
image = "postgres-mcp:latest"
port = 5432
digest = "abc123"
`)
			},
			wantCount: 1,
			wantKeys:  []containerKey{{name: "postgres", digest: "abc123"}},
			wantPorts: map[containerKey]int{{name: "postgres", digest: "abc123"}: 5432},
		},
		"multiple containers": {
			seed: func(t *testing.T, st store.Store) {
				seedOCIStore(t, st, "postgres", "aaa", `
transport = "http"
name = "postgres"
image = "pg:latest"
port = 5432
digest = "aaa"
`)
				seedOCIStore(t, st, "redis", "bbb", `
transport = "http"
name = "redis"
image = "redis:latest"
port = 6379
digest = "bbb"
`)
			},
			wantCount: 2,
			wantKeys:  []containerKey{{name: "postgres", digest: "aaa"}, {name: "redis", digest: "bbb"}},
		},
		"default port when omitted": {
			seed: func(t *testing.T, st store.Store) {
				seedOCIStore(t, st, "server", "eee", `
transport = "http"
name = "server"
image = "mcp:latest"
digest = "eee"
`)
			},
			wantCount: 1,
			wantPorts: map[containerKey]int{{name: "server", digest: "eee"}: 8080},
		},
		"skips entries without image": {
			seed: func(t *testing.T, st store.Store) {
				seedOCIStore(t, st, "broken", "fff", `
transport = "http"
name = "broken"
`)
			},
			wantCount: 0,
		},
		"empty oci directory": {
			seed:      func(t *testing.T, st store.Store) {},
			wantCount: 0,
		},
		"same name different digests": {
			seed: func(t *testing.T, st store.Store) {
				seedOCIStore(t, st, "postgres", "digest-v1", `
transport = "http"
name = "postgres"
image = "pg:v1"
port = 5432
digest = "digest-v1"
`)
				seedOCIStore(t, st, "postgres", "digest-v2", `
transport = "http"
name = "postgres"
image = "pg:v2"
port = 5432
digest = "digest-v2"
`)
			},
			wantCount: 2,
			wantKeys: []containerKey{
				{name: "postgres", digest: "digest-v1"},
				{name: "postgres", digest: "digest-v2"},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			st := store.New(t.TempDir())
			tc.seed(t, st)

			srv, err := NewServerFromStore(st, DefaultPort, &container.Engine{})
			if err != nil {
				t.Fatalf("NewServerFromStore() error: %v", err)
			}
			if len(srv.Containers) != tc.wantCount {
				t.Errorf("container count = %d, want %d", len(srv.Containers), tc.wantCount)
			}
			for _, key := range tc.wantKeys {
				if _, ok := srv.Containers[key]; !ok {
					t.Errorf("missing container key {name: %q, digest: %q}", key.name, key.digest)
				}
			}
			for key, port := range tc.wantPorts {
				mc, ok := srv.Containers[key]
				if !ok {
					t.Errorf("missing container key {name: %q, digest: %q} for port check", key.name, key.digest)
					continue
				}
				if mc.containerPort != port {
					t.Errorf("container %q port = %d, want %d", key.name, mc.containerPort, port)
				}
			}
		})
	}
}

func TestProxyHandlerMissingHeader(t *testing.T) {
	srv := &Server{
		Containers: map[containerKey]*managedContainer{},
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.proxyHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestProxyHandlerUnknownServer(t *testing.T) {
	srv := &Server{
		Containers: map[containerKey]*managedContainer{
			{name: "known", digest: "abc"}: {name: "known", image: "img:latest"},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(MCPServerHeader, "unknown")
	req.Header.Set(MCPServerDigestHeader, "xyz")
	rec := httptest.NewRecorder()
	srv.proxyHandler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestProxyHandlerWrongDigest(t *testing.T) {
	srv := &Server{
		Containers: map[containerKey]*managedContainer{
			{name: "postgres", digest: "abc"}: {name: "postgres", image: "pg:latest"},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(MCPServerHeader, "postgres")
	req.Header.Set(MCPServerDigestHeader, "wrong-digest")
	rec := httptest.NewRecorder()
	srv.proxyHandler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
