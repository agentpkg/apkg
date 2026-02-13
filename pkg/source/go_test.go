package source

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentpkg/agentpkg/pkg/config"
	"github.com/agentpkg/agentpkg/pkg/store"
)

// requireGo skips the test if go is not available.
func requireGo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not found in PATH")
	}
}

func TestGoSourceImplementsSource(t *testing.T) {
	var _ Source = &GoSource{}
}

func TestGoModulePath(t *testing.T) {
	tests := map[string]struct {
		pkg  string
		want string
	}{
		"module with version": {
			pkg:  "github.com/go-delve/mcp-dap-server@main",
			want: "github.com/go-delve/mcp-dap-server",
		},
		"module with semver": {
			pkg:  "github.com/example/tool@v1.2.3",
			want: "github.com/example/tool",
		},
		"module with latest": {
			pkg:  "github.com/example/tool@latest",
			want: "github.com/example/tool",
		},
		"module without version": {
			pkg:  "github.com/example/tool",
			want: "github.com/example/tool",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := &GoSource{Package: tc.pkg}
			got := s.modulePath()
			if got != tc.want {
				t.Errorf("modulePath() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGoVersionSuffix(t *testing.T) {
	tests := map[string]struct {
		pkg  string
		want string
	}{
		"branch ref": {
			pkg:  "github.com/go-delve/mcp-dap-server@main",
			want: "main",
		},
		"semver": {
			pkg:  "github.com/example/tool@v1.2.3",
			want: "v1.2.3",
		},
		"latest": {
			pkg:  "github.com/example/tool@latest",
			want: "latest",
		},
		"no version": {
			pkg:  "github.com/example/tool",
			want: "latest",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := &GoSource{Package: tc.pkg}
			got := s.versionSuffix()
			if got != tc.want {
				t.Errorf("versionSuffix() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGoGetStoreSegments(t *testing.T) {
	tests := map[string]struct {
		pkg     string
		version string
		want    []string
	}{
		"standard module": {
			pkg:     "github.com/go-delve/mcp-dap-server@main",
			version: "v0.0.0-20250515123456-abcdef123456",
			want:    []string{"go", "github.com", "go-delve", "mcp-dap-server", "v0.0.0-20250515123456-abcdef123456"},
		},
		"two segment module": {
			pkg:     "github.com/tool@v1.0.0",
			version: "v1.0.0",
			want:    []string{"go", "github.com", "tool", "v1.0.0"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := &GoSource{Package: tc.pkg}
			got := s.getStoreSegments(tc.version)
			if len(got) != len(tc.want) {
				t.Fatalf("getStoreSegments() = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("getStoreSegments()[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestGoWriteMCPConfig(t *testing.T) {
	tests := map[string]struct {
		mcpConfig config.MCPSource
		segs      []string
	}{
		"stdio transport": {
			mcpConfig: config.MCPSource{Transport: "stdio"},
			segs:      []string{"go", "github.com", "example", "tool", "v1.0.0"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			st := store.New(t.TempDir())
			st.EnsureDir(tc.segs...)

			s := &GoSource{MCPConfig: tc.mcpConfig}
			err := s.writeMCPConfig(st, tc.segs)
			if err != nil {
				t.Fatalf("writeMCPConfig() error: %v", err)
			}

			mcpSegs := append(tc.segs, mcpFileName)
			data, err := st.ReadFile(mcpSegs...)
			if err != nil {
				t.Fatalf("reading mcp config: %v", err)
			}
			if len(data) == 0 {
				t.Error("mcp config file is empty")
			}
		})
	}
}

func TestGoWriteMCPConfigFilePerms(t *testing.T) {
	st := store.New(t.TempDir())
	segs := []string{"go", "github.com", "example", "tool", "v1.0.0"}
	st.EnsureDir(segs...)

	s := &GoSource{
		MCPConfig: config.MCPSource{Transport: "stdio"},
	}

	if err := s.writeMCPConfig(st, segs); err != nil {
		t.Fatalf("writeMCPConfig() error: %v", err)
	}

	mcpPath := filepath.Join(st.Path(segs...), mcpFileName)
	info, err := os.Stat(mcpPath)
	if err != nil {
		t.Fatalf("stat mcp config: %v", err)
	}

	gotPerms := info.Mode().Perm()
	if gotPerms != mcpFilePerms {
		t.Errorf("mcp config perms = %o, want %o", gotPerms, mcpFilePerms)
	}
}

func TestGoFetch(t *testing.T) {
	requireGo(t)

	tests := map[string]struct {
		pkg       string
		mcpConfig config.MCPSource
	}{
		"pinned version": {
			pkg:       "github.com/charmbracelet/gum@v0.14.5",
			mcpConfig: config.MCPSource{Transport: "stdio"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := store.New(t.TempDir())
			src := &GoSource{
				Package:   tc.pkg,
				MCPConfig: tc.mcpConfig,
			}

			result, err := src.Fetch(context.Background(), s)
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

			// mcp.toml should have been written
			mcpPath := filepath.Join(result.Dir, mcpFileName)
			if _, err := os.Stat(mcpPath); err != nil {
				t.Errorf("expected %s in %q: %v", mcpFileName, result.Dir, err)
			}

			// bin/ directory should exist with the binary
			binPath := filepath.Join(result.Dir, "bin", "gum")
			if _, err := os.Stat(binPath); err != nil {
				t.Errorf("expected binary at %q: %v", binPath, err)
			}
		})
	}
}

func TestGoFetchCached(t *testing.T) {
	requireGo(t)

	s := store.New(t.TempDir())
	src := &GoSource{
		Package:   "github.com/charmbracelet/gum@v0.14.5",
		MCPConfig: config.MCPSource{Transport: "stdio"},
	}

	first, err := src.Fetch(context.Background(), s)
	if err != nil {
		t.Fatalf("first Fetch() error: %v", err)
	}

	second, err := src.Fetch(context.Background(), s)
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

func TestGoFetchContextCanceled(t *testing.T) {
	requireGo(t)

	s := store.New(t.TempDir())
	src := &GoSource{
		Package:   "github.com/charmbracelet/gum@v0.14.5",
		MCPConfig: config.MCPSource{Transport: "stdio"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := src.Fetch(ctx, s)
	if err == nil {
		t.Fatal("expected error with canceled context, got nil")
	}
}
