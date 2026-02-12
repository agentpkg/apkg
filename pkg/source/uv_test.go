package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentpkg/agentpkg/pkg/config"
	"github.com/agentpkg/agentpkg/pkg/store"
)

func TestUVSourceImplementsSource(t *testing.T) {
	var _ Source = &UVSource{}
}

func TestUVPackageName(t *testing.T) {
	tests := map[string]struct {
		pkg  string
		want string
	}{
		"plain package": {
			pkg:  "mcp-server-git",
			want: "mcp-server-git",
		},
		"package with version": {
			pkg:  "mcp-server-git==1.2.3",
			want: "mcp-server-git",
		},
		"package with complex version": {
			pkg:  "my-tool==0.1.0rc1",
			want: "my-tool",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := &UVSource{Package: tc.pkg}
			got := s.packageName()
			if got != tc.want {
				t.Errorf("packageName() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUVGetStoreSegments(t *testing.T) {
	tests := map[string]struct {
		pkg     string
		version string
		want    []string
	}{
		"plain package": {
			pkg:     "mcp-server-git",
			version: "1.2.3",
			want:    []string{"uv", "mcp-server-git", "1.2.3"},
		},
		"package with version spec": {
			pkg:     "mcp-server-git==1.2.3",
			version: "1.2.3",
			want:    []string{"uv", "mcp-server-git", "1.2.3"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := &UVSource{Package: tc.pkg}
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

func TestUVResolveConcreteVersionPinned(t *testing.T) {
	tests := map[string]struct {
		pkg         string
		wantVersion string
	}{
		"pinned version": {
			pkg:         "mcp-server-git==2.1.0",
			wantVersion: "2.1.0",
		},
		"pinned pre-release": {
			pkg:         "my-tool==0.1.0rc1",
			wantVersion: "0.1.0rc1",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			src := &UVSource{Package: tc.pkg}
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

func TestUVResolveConcreteVersionPyPI(t *testing.T) {
	tests := map[string]struct {
		pkg         string
		pypiVersion string
		statusCode  int
		wantErr     bool
	}{
		"latest version from pypi": {
			pkg:         "mcp-server-git",
			pypiVersion: "3.0.0",
			statusCode:  http.StatusOK,
		},
		"pypi returns error": {
			pkg:        "nonexistent-pkg",
			statusCode: http.StatusNotFound,
			wantErr:    true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.statusCode != http.StatusOK {
					w.WriteHeader(tc.statusCode)
					return
				}
				resp := struct {
					Info struct {
						Version string `json:"version"`
					} `json:"info"`
				}{}
				resp.Info.Version = tc.pypiVersion
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()

			src := &uvSourceWithCustomURL{
				UVSource: UVSource{Package: tc.pkg},
				baseURL:  server.URL,
			}
			got, err := src.resolveConcreteVersion(context.Background())
			if (err != nil) != tc.wantErr {
				t.Fatalf("resolveConcreteVersion() error = %v, wantErr = %v", err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.pypiVersion {
				t.Errorf("resolveConcreteVersion() = %q, want %q", got, tc.pypiVersion)
			}
		})
	}
}

// uvSourceWithCustomURL wraps UVSource to allow testing with a custom PyPI URL.
type uvSourceWithCustomURL struct {
	UVSource
	baseURL string
}

func (s *uvSourceWithCustomURL) resolveConcreteVersion(ctx context.Context) (string, error) {
	if idx := strings.Index(s.Package, "=="); idx >= 0 {
		return s.Package[idx+2:], nil
	}

	url := s.baseURL + "/pypi/" + s.packageName() + "/json"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("pypi returned status %d for %s", resp.StatusCode, s.packageName())
	}

	var result struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.Info.Version == "" {
		return "", fmt.Errorf("no version found for %s on pypi", s.packageName())
	}

	return result.Info.Version, nil
}

func TestUVWriteMCPConfig(t *testing.T) {
	tests := map[string]struct {
		mcpConfig config.MCPSource
		segs      []string
	}{
		"stdio transport": {
			mcpConfig: config.MCPSource{Transport: "stdio"},
			segs:      []string{"uv", "some-pkg", "1.0.0"},
		},
		"http transport": {
			mcpConfig: config.MCPSource{Transport: "http"},
			segs:      []string{"uv", "some-pkg", "2.0.0"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			st := store.New(t.TempDir())
			st.EnsureDir(tc.segs...)

			s := &UVSource{MCPConfig: tc.mcpConfig}
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

func TestUVWriteMCPConfigFilePerms(t *testing.T) {
	st := store.New(t.TempDir())
	segs := []string{"uv", "test-pkg", "1.0.0"}
	st.EnsureDir(segs...)

	s := &UVSource{
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

// requireUV skips the test if uv is not available.
func requireUV(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("uv"); err != nil {
		t.Skip("uv not found in PATH")
	}
}

func TestUVFetch(t *testing.T) {
	requireUV(t)

	tests := map[string]struct {
		pkg       string
		mcpConfig config.MCPSource
	}{
		"pinned package": {
			pkg:       "mcp-server-git==2026.1.14",
			mcpConfig: config.MCPSource{Transport: "stdio"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := store.New(t.TempDir())
			src := &UVSource{
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

			// .venv should exist
			venvPath := filepath.Join(result.Dir, ".venv")
			if _, err := os.Stat(venvPath); err != nil {
				t.Errorf("expected .venv in %q: %v", result.Dir, err)
			}
		})
	}
}

func TestUVFetchCached(t *testing.T) {
	requireUV(t)

	s := store.New(t.TempDir())
	src := &UVSource{
		Package:   "mcp-server-git==2026.1.14",
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

func TestUVFetchContextCanceled(t *testing.T) {
	requireUV(t)

	s := store.New(t.TempDir())
	src := &UVSource{
		Package:   "mcp-server-git==2026.1.14",
		MCPConfig: config.MCPSource{Transport: "stdio"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := src.Fetch(ctx, s)
	if err == nil {
		t.Fatal("expected error with canceled context, got nil")
	}
}
