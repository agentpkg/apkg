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

// requireNPM skips the test if npm is not available.
func requireNPM(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm not found in PATH")
	}
}

func TestPackageName(t *testing.T) {
	tests := map[string]struct {
		pkg  string
		want string
	}{
		"plain package": {
			pkg:  "some-mcp-server",
			want: "some-mcp-server",
		},
		"package with version": {
			pkg:  "some-mcp-server@1.2.3",
			want: "some-mcp-server",
		},
		"package with version range": {
			pkg:  "some-mcp-server@^1.0.0",
			want: "some-mcp-server",
		},
		"scoped package": {
			pkg:  "@modelcontextprotocol/inspector",
			want: "@modelcontextprotocol/inspector",
		},
		"scoped package with version": {
			pkg:  "@modelcontextprotocol/inspector@1.0.0",
			want: "@modelcontextprotocol/inspector",
		},
		"package with latest tag": {
			pkg:  "some-mcp-server@latest",
			want: "some-mcp-server",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := &NPMSource{Package: tc.pkg}
			got := s.packageName()
			if got != tc.want {
				t.Errorf("packageName() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGetStoreSegments(t *testing.T) {
	tests := map[string]struct {
		pkg     string
		version string
		want    []string
	}{
		"plain package": {
			pkg:     "some-mcp-server",
			version: "1.2.3",
			want:    []string{"npm", "some-mcp-server", "1.2.3"},
		},
		"scoped package": {
			pkg:     "@modelcontextprotocol/inspector",
			version: "2.0.0",
			want:    []string{"npm", "@modelcontextprotocol", "inspector", "2.0.0"},
		},
		"scoped package with version tag": {
			pkg:     "@modelcontextprotocol/inspector@1.0.0",
			version: "1.0.0",
			want:    []string{"npm", "@modelcontextprotocol", "inspector", "1.0.0"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := &NPMSource{Package: tc.pkg}
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

func TestWriteMCPConfig(t *testing.T) {
	tests := map[string]struct {
		mcpConfig config.MCPSource
		segs      []string
		wantErr   bool
	}{
		"stdio transport": {
			mcpConfig: config.MCPSource{Transport: "stdio"},
			segs:      []string{"npm", "some-pkg", "1.0.0"},
		},
		"http transport": {
			mcpConfig: config.MCPSource{Transport: "http"},
			segs:      []string{"npm", "some-pkg", "2.0.0"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			st := store.New(t.TempDir())
			st.EnsureDir(tc.segs...)

			s := &NPMSource{MCPConfig: tc.mcpConfig}
			err := s.writeMCPConfig(st, tc.segs)
			if (err != nil) != tc.wantErr {
				t.Fatalf("writeMCPConfig() error = %v, wantErr = %v", err, tc.wantErr)
			}

			if err == nil {
				mcpSegs := append(tc.segs, mcpFileName)
				data, err := st.ReadFile(mcpSegs...)
				if err != nil {
					t.Fatalf("reading mcp config: %v", err)
				}
				if len(data) == 0 {
					t.Error("mcp config file is empty")
				}
			}
		})
	}
}

func TestWriteMCPConfigContent(t *testing.T) {
	st := store.New(t.TempDir())
	segs := []string{"npm", "test-pkg", "1.0.0"}
	st.EnsureDir(segs...)

	s := &NPMSource{
		MCPConfig: config.MCPSource{Transport: "stdio"},
	}

	if err := s.writeMCPConfig(st, segs); err != nil {
		t.Fatalf("writeMCPConfig() error: %v", err)
	}

	mcpSegs := append(segs, mcpFileName)
	data, err := st.ReadFile(mcpSegs...)
	if err != nil {
		t.Fatalf("reading mcp config: %v", err)
	}

	content := string(data)
	if content == "" {
		t.Error("expected non-empty config content")
	}
}

func TestWriteMCPConfigFilePerms(t *testing.T) {
	st := store.New(t.TempDir())
	segs := []string{"npm", "test-pkg", "1.0.0"}
	st.EnsureDir(segs...)

	s := &NPMSource{
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

func TestNPMSourceImplementsSource(t *testing.T) {
	var _ Source = &NPMSource{}
}

func TestNPMFetch(t *testing.T) {
	requireNPM(t)

	tests := map[string]struct {
		pkg       string
		mcpConfig config.MCPSource
	}{
		"plain package": {
			pkg:       "is-number@7.0.0",
			mcpConfig: config.MCPSource{Transport: "stdio"},
		},
		"scoped package": {
			pkg:       "@anthropic-ai/tokenizer@0.0.3",
			mcpConfig: config.MCPSource{Transport: "stdio"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := store.New(t.TempDir())
			src := &NPMSource{
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
		})
	}
}

func TestNPMFetchCached(t *testing.T) {
	requireNPM(t)

	s := store.New(t.TempDir())
	src := &NPMSource{
		Package:   "is-number@7.0.0",
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

func TestNPMFetchContextCanceled(t *testing.T) {
	requireNPM(t)

	s := store.New(t.TempDir())
	src := &NPMSource{
		Package:   "is-number@7.0.0",
		MCPConfig: config.MCPSource{Transport: "stdio"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := src.Fetch(ctx, s)
	if err == nil {
		t.Fatal("expected error with canceled context, got nil")
	}
}

func TestNPMResolveConcreteVersion(t *testing.T) {
	requireNPM(t)

	tests := map[string]struct {
		pkg         string
		wantVersion string
	}{
		"pinned version": {
			pkg:         "is-number@7.0.0",
			wantVersion: "7.0.0",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			src := &NPMSource{Package: tc.pkg}
			got, err := src.resolveConcreteVersion(context.Background())
			if err != nil {
				t.Fatalf("resolveConcreteVersion() error: %v", err)
			}
			if got != tc.wantVersion {
				t.Errorf("resolveConcreteVersion() = %q, want %q", got, tc.wantVersion)
			}
		})
	}
}

func TestNPMResolveConcreteVersionNotFound(t *testing.T) {
	requireNPM(t)

	src := &NPMSource{Package: "this-package-should-not-exist-ever-abc123xyz"}
	_, err := src.resolveConcreteVersion(context.Background())
	if err == nil {
		t.Fatal("expected error for nonexistent package, got nil")
	}
}
