package cursor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentpkg/agentpkg/pkg/projector"
)

func TestSupportsSkills(t *testing.T) {
	c := &cursorProjector{}
	if !c.SupportsSkills() {
		t.Error("SupportsSkills() = false, want true")
	}
}

func TestUnprojectMCPServers(t *testing.T) {
	tests := map[string]struct {
		scope       projector.Scope
		initialJSON map[string]any
		names       []string
		verify      func(t *testing.T, config map[string]any)
	}{
		"removes global server": {
			scope: projector.ScopeGlobal,
			initialJSON: map[string]any{
				"mcpServers": map[string]any{
					"my-server": map[string]any{"command": "test"},
					"keep":      map[string]any{"command": "keep"},
				},
			},
			names: []string{"my-server"},
			verify: func(t *testing.T, config map[string]any) {
				servers := config["mcpServers"].(map[string]any)
				if _, ok := servers["my-server"]; ok {
					t.Error("expected my-server to be removed")
				}
				if _, ok := servers["keep"]; !ok {
					t.Error("expected keep to remain")
				}
			},
		},
		"removes project-scoped server": {
			scope: projector.ScopeLocal,
			initialJSON: map[string]any{
				"mcpServers": map[string]any{
					"my-server": map[string]any{"command": "test"},
					"keep":      map[string]any{"command": "keep"},
				},
			},
			names: []string{"my-server"},
			verify: func(t *testing.T, config map[string]any) {
				servers := config["mcpServers"].(map[string]any)
				if _, ok := servers["my-server"]; ok {
					t.Error("expected my-server to be removed")
				}
				if _, ok := servers["keep"]; !ok {
					t.Error("expected keep to remain")
				}
			},
		},
		"nonexistent server is a no-op": {
			scope: projector.ScopeGlobal,
			initialJSON: map[string]any{
				"mcpServers": map[string]any{
					"keep": map[string]any{"command": "keep"},
				},
			},
			names: []string{"does-not-exist"},
			verify: func(t *testing.T, config map[string]any) {
				servers := config["mcpServers"].(map[string]any)
				if _, ok := servers["keep"]; !ok {
					t.Error("expected keep to remain")
				}
			},
		},
		"no mcpServers key is a no-op": {
			scope:       projector.ScopeGlobal,
			initialJSON: map[string]any{},
			names:       []string{"anything"},
			verify:      func(t *testing.T, config map[string]any) {},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var configPath string
			if tc.scope == projector.ScopeGlobal {
				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				configPath = filepath.Join(homeDir, ".cursor", "mcp.json")
			} else {
				projectDir := t.TempDir()
				configPath = filepath.Join(projectDir, ".cursor", "mcp.json")

				// Write initial config and run unprojection.
				if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
					t.Fatal(err)
				}
				data, _ := json.Marshal(tc.initialJSON)
				if err := os.WriteFile(configPath, data, 0644); err != nil {
					t.Fatal(err)
				}

				c := &cursorProjector{}
				opts := projector.ProjectionOpts{
					ProjectDir: projectDir,
					Scope:      tc.scope,
				}
				if err := c.UnprojectMCPServers(opts, tc.names); err != nil {
					t.Fatalf("UnprojectMCPServers() error = %v", err)
				}

				result, err := os.ReadFile(configPath)
				if err != nil {
					t.Fatal(err)
				}
				var config map[string]any
				if err := json.Unmarshal(result, &config); err != nil {
					t.Fatal(err)
				}
				tc.verify(t, config)
				return
			}

			if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
				t.Fatal(err)
			}
			data, _ := json.Marshal(tc.initialJSON)
			if err := os.WriteFile(configPath, data, 0644); err != nil {
				t.Fatal(err)
			}

			c := &cursorProjector{}
			opts := projector.ProjectionOpts{
				ProjectDir: t.TempDir(),
				Scope:      tc.scope,
			}
			if err := c.UnprojectMCPServers(opts, tc.names); err != nil {
				t.Fatalf("UnprojectMCPServers() error = %v", err)
			}

			result, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			var config map[string]any
			if err := json.Unmarshal(result, &config); err != nil {
				t.Fatal(err)
			}
			tc.verify(t, config)
		})
	}
}
