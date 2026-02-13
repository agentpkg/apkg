package source

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentpkg/agentpkg/pkg/config"
	"github.com/agentpkg/agentpkg/pkg/store"
)

func TestStaticSourceImplementsSource(t *testing.T) {
	var _ Source = &StaticSource{}
}

func TestStaticSourceFetch(t *testing.T) {
	tests := map[string]struct {
		name      string
		mcpConfig config.MCPSource
	}{
		"unmanaged stdio": {
			name: "my-server",
			mcpConfig: config.MCPSource{
				Transport:               "stdio",
				UnmanagedStdioMCPConfig: &config.UnmanagedStdioMCPConfig{Command: "/usr/bin/echo"},
				LocalMCPConfig:          &config.LocalMCPConfig{Args: []string{"hello"}},
			},
		},
		"external http": {
			name: "remote-server",
			mcpConfig: config.MCPSource{
				Transport:            "http",
				ExternalHttpMCPConfig: &config.ExternalHttpMCPConfig{URL: "https://example.com/mcp"},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			st := store.New(t.TempDir())
			src := &StaticSource{
				Name:      tc.name,
				MCPConfig: tc.mcpConfig,
			}

			result, err := src.Fetch(context.Background(), st)
			if err != nil {
				t.Fatalf("Fetch() error: %v", err)
			}

			if result.Dir == "" {
				t.Error("Dir is empty")
			}

			info, err := os.Stat(result.Dir)
			if err != nil {
				t.Fatalf("Dir %q does not exist: %v", result.Dir, err)
			}
			if !info.IsDir() {
				t.Fatalf("Dir %q is not a directory", result.Dir)
			}

			if !strings.HasPrefix(result.Integrity, "sha256:") {
				t.Errorf("Integrity = %q, want sha256: prefix", result.Integrity)
			}

			mcpPath := filepath.Join(result.Dir, mcpFileName)
			if _, err := os.Stat(mcpPath); err != nil {
				t.Errorf("expected %s in %q: %v", mcpFileName, result.Dir, err)
			}
		})
	}
}

func TestStaticSourceFetchCached(t *testing.T) {
	st := store.New(t.TempDir())
	src := &StaticSource{
		Name: "my-server",
		MCPConfig: config.MCPSource{
			Transport:              "stdio",
			UnmanagedStdioMCPConfig: &config.UnmanagedStdioMCPConfig{Command: "/usr/bin/echo"},
		},
	}

	first, err := src.Fetch(context.Background(), st)
	if err != nil {
		t.Fatalf("first Fetch() error: %v", err)
	}

	second, err := src.Fetch(context.Background(), st)
	if err != nil {
		t.Fatalf("second Fetch() error: %v", err)
	}

	if first.Dir != second.Dir {
		t.Errorf("Dir mismatch: %q vs %q", first.Dir, second.Dir)
	}
	if first.Integrity != second.Integrity {
		t.Errorf("Integrity mismatch: %q vs %q", first.Integrity, second.Integrity)
	}
}

func TestStaticSourceDifferentConfigsDontCollide(t *testing.T) {
	st := store.New(t.TempDir())

	src1 := &StaticSource{
		Name: "my-server",
		MCPConfig: config.MCPSource{
			Transport:              "stdio",
			UnmanagedStdioMCPConfig: &config.UnmanagedStdioMCPConfig{Command: "/usr/bin/echo"},
		},
	}

	src2 := &StaticSource{
		Name: "my-server",
		MCPConfig: config.MCPSource{
			Transport:              "stdio",
			UnmanagedStdioMCPConfig: &config.UnmanagedStdioMCPConfig{Command: "/usr/local/bin/other"},
		},
	}

	r1, err := src1.Fetch(context.Background(), st)
	if err != nil {
		t.Fatalf("first Fetch() error: %v", err)
	}

	r2, err := src2.Fetch(context.Background(), st)
	if err != nil {
		t.Fatalf("second Fetch() error: %v", err)
	}

	if r1.Dir == r2.Dir {
		t.Error("expected different dirs for different configs with the same name")
	}
}

func TestStaticSourceStoreSegments(t *testing.T) {
	src := &StaticSource{
		Name: "test-server",
		MCPConfig: config.MCPSource{
			Transport:              "stdio",
			UnmanagedStdioMCPConfig: &config.UnmanagedStdioMCPConfig{Command: "/bin/test"},
		},
	}

	segs := src.storeSegments([]byte("test-data"))
	if len(segs) != 3 {
		t.Fatalf("storeSegments() returned %d segments, want 3", len(segs))
	}
	if segs[0] != "static" {
		t.Errorf("segs[0] = %q, want %q", segs[0], "static")
	}
	if segs[1] != "test-server" {
		t.Errorf("segs[1] = %q, want %q", segs[1], "test-server")
	}
	if len(segs[2]) != 64 {
		t.Errorf("segs[2] hash length = %d, want 64", len(segs[2]))
	}
}
