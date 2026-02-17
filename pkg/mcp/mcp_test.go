package mcp

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agentpkg/agentpkg/pkg/container"
)

func TestLoad(t *testing.T) {
	tests := map[string]struct {
		files    map[string]string
		wantName string
		wantType string // "stdio" or "http"
		wantCmd  string // for stdio (relative to dir for managed, absolute for unmanaged check)
		wantURL  string // for http
		wantArgs []string
		wantEnv  map[string]string
		wantErr  bool
	}{
		"unmanaged stdio": {
			files: map[string]string{
				"mcp.toml": `
name = "simple"
command = "echo"
args = ["hello"]
env = { FOO = "bar" }
`,
			},
			wantName: "simple",
			wantType: "stdio",
			wantCmd:  "echo",
			wantArgs: []string{"hello"},
			wantEnv:  map[string]string{"FOO": "bar"},
		},
		"managed npm single bin string": {
			files: map[string]string{
				"mcp.toml": `
name = "npm-single"
package = "npm:my-pkg"
runtime = "/usr/local/bin/node"
`,
				"node_modules/my-pkg/package.json": `{"bin": "cli.js"}`,
			},
			wantName: "npm-single",
			wantType: "stdio",
			wantCmd:  "/usr/local/bin/node",
			wantArgs: []string{filepath.Join("node_modules", ".bin", "my-pkg")},
		},
		"managed npm bin map single": {
			files: map[string]string{
				"mcp.toml": `
name = "npm-map"
package = "npm:my-pkg-map"
runtime = "/usr/local/bin/node"
`,
				"node_modules/my-pkg-map/package.json": `{"bin": {"my-cli": "cli.js"}}`,
			},
			wantName: "npm-map",
			wantType: "stdio",
			wantCmd:  "/usr/local/bin/node",
			wantArgs: []string{filepath.Join("node_modules", ".bin", "my-cli")},
		},
		"managed npm bin map multi match unscoped": {
			files: map[string]string{
				"mcp.toml": `
name = "npm-multi"
package = "npm:@scope/my-pkg"
runtime = "/usr/local/bin/node"
`,
				"node_modules/@scope/my-pkg/package.json": `{"bin": {"other": "other.js", "my-pkg": "cli.js"}}`,
			},
			wantName: "npm-multi",
			wantType: "stdio",
			wantCmd:  "/usr/local/bin/node",
			wantArgs: []string{filepath.Join("node_modules", ".bin", "my-pkg")},
		},
		"managed npm with user args": {
			files: map[string]string{
				"mcp.toml": `
name = "npm-args"
package = "npm:my-pkg"
runtime = "/usr/local/bin/node"
args = ["--stdio"]
`,
				"node_modules/my-pkg/package.json": `{"bin": "cli.js"}`,
			},
			wantName: "npm-args",
			wantType: "stdio",
			wantCmd:  "/usr/local/bin/node",
			wantArgs: []string{filepath.Join("node_modules", ".bin", "my-pkg"), "--stdio"},
		},
		"managed uv": {
			files: map[string]string{
				"mcp.toml": `
name = "uv-test"
package = "uv:my-uv-pkg"
`,
				".venv/bin/my-uv-pkg": "executable content",
			},
			wantName: "uv-test",
			wantType: "stdio",
			wantCmd:  filepath.Join(".venv", "bin", "my-uv-pkg"),
		},
		"managed go": {
			files: map[string]string{
				"mcp.toml": `
name = "go-test"
package = "go:github.com/example/my-tool"
`,
				"bin/my-tool": "executable content",
			},
			wantName: "go-test",
			wantType: "stdio",
			wantCmd:  filepath.Join("bin", "my-tool"),
		},
		"managed go scoped": {
			files: map[string]string{
				"mcp.toml": `
name = "go-scoped"
package = "go:github.com/go-delve/mcp-dap-server@main"
`,
				"bin/mcp-dap-server": "executable content",
			},
			wantName: "go-scoped",
			wantType: "stdio",
			wantCmd:  filepath.Join("bin", "mcp-dap-server"),
		},
		"external http": {
			files: map[string]string{
				"mcp.toml": `
name = "http-test"
url = "http://example.com"
transport = "sse"
`,
			},
			wantName: "http-test",
			wantType: "sse",
			wantURL:  "http://example.com",
		},
		"container http": {
			files: map[string]string{
				"mcp.toml": `
name = "container-test"
image = "my-image"
digest = "sha256:abc"
path = "/mcp"
`,
			},
			wantName: "container-test",
			wantType: "http",
			wantURL:  "http://localhost:19513/mcp",
		},
		"error missing config": {
			files:   map[string]string{},
			wantErr: true,
		},
		"error invalid toml": {
			files: map[string]string{
				"mcp.toml": `invalid = toml`,
			},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dir := setupDir(t, tc.files)
			server, err := Load(dir)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Load() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}

			if server.Name() != tc.wantName {
				t.Errorf("Name() = %v, want %v", server.Name(), tc.wantName)
			}
			if server.Transport() != tc.wantType {
				t.Errorf("Transport() = %v, want %v", server.Transport(), tc.wantType)
			}

			if tc.wantCmd != "" {
				got := server.Command()
				// Case 1: Unmanaged exact match
				if got == tc.wantCmd {
					// OK
				} else {
					// Case 2: Managed absolute path
					expectedAbs := filepath.Join(dir, tc.wantCmd)
					if got != expectedAbs {
						t.Errorf("Command() = %v, want %v or %v", got, tc.wantCmd, expectedAbs)
					}
				}
			}

			if tc.wantURL != "" {
				if server.URL() != tc.wantURL {
					t.Errorf("URL() = %v, want %v", server.URL(), tc.wantURL)
				}
			}

				if len(tc.wantArgs) > 0 {
				got := server.Args()
				if !reflect.DeepEqual(got, tc.wantArgs) {
					// Try with dir-prefixed relative paths for managed packages.
					// Only prefix args that look like paths (contain a separator).
					prefixed := make([]string, len(tc.wantArgs))
					for i, a := range tc.wantArgs {
						if filepath.IsAbs(a) || !strings.Contains(a, string(filepath.Separator)) {
							prefixed[i] = a
						} else {
							prefixed[i] = filepath.Join(dir, a)
						}
					}
					if !reflect.DeepEqual(got, prefixed) {
						t.Errorf("Args() = %v, want %v or %v", got, tc.wantArgs, prefixed)
					}
				}
			}

			if len(tc.wantEnv) > 0 {
				if !reflect.DeepEqual(server.Env(), tc.wantEnv) {
					t.Errorf("Env() = %v, want %v", server.Env(), tc.wantEnv)
				}
			}
		})
	}
}

func TestLoadContainerStdio(t *testing.T) {
	// Stub the container engine detection so tests don't require docker/podman.
	orig := detectContainerEngine
	t.Cleanup(func() { detectContainerEngine = orig })
	detectContainerEngine = func() (*container.Engine, error) {
		return &container.Engine{Path: "/usr/bin/docker", Name: "docker"}, nil
	}

	tests := map[string]struct {
		files    map[string]string
		wantName string
		wantCmd  string
		wantArgs []string
	}{
		"basic": {
			files: map[string]string{
				"mcp.toml": `
name = "stdio-container"
transport = "stdio"
image = "my-image:latest"
digest = "abc123"
`,
			},
			wantName: "stdio-container",
			wantCmd:  "/usr/bin/docker",
			wantArgs: []string{"run", "--rm", "-i", "my-image:latest@sha256:abc123"},
		},
		"with volumes and env and args and network": {
			files: map[string]string{
				"mcp.toml": `
name = "full-container"
transport = "stdio"
image = "my-image:latest"
digest = "def456"
volumes = ["/host/data:/data:ro"]
network = "host"
args = ["--verbose"]
env = { API_KEY = "secret" }
`,
			},
			wantName: "full-container",
			wantCmd:  "/usr/bin/docker",
			wantArgs: []string{
				"run", "--rm", "-i",
				"--network", "host",
				"-v", "/host/data:/data:ro",
				"-e", "API_KEY=secret",
				"my-image:latest@sha256:def456",
				"--verbose",
			},
		},
		"no digest": {
			files: map[string]string{
				"mcp.toml": `
name = "no-digest"
transport = "stdio"
image = "my-image:latest"
`,
			},
			wantName: "no-digest",
			wantCmd:  "/usr/bin/docker",
			wantArgs: []string{"run", "--rm", "-i", "my-image:latest"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dir := setupDir(t, tc.files)
			server, err := Load(dir)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			if server.Name() != tc.wantName {
				t.Errorf("Name() = %v, want %v", server.Name(), tc.wantName)
			}
			if server.Transport() != "stdio" {
				t.Errorf("Transport() = %v, want stdio", server.Transport())
			}
			if server.Command() != tc.wantCmd {
				t.Errorf("Command() = %v, want %v", server.Command(), tc.wantCmd)
			}
			if !reflect.DeepEqual(server.Args(), tc.wantArgs) {
				t.Errorf("Args() = %v, want %v", server.Args(), tc.wantArgs)
			}
		})
	}
}

func setupDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
