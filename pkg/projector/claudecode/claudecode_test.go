package claudecode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentpkg/agentpkg/pkg/projector"
)

func TestSupportsSkills(t *testing.T) {
	c := &claudeCodeProjector{}
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
				"projects": map[string]any{},
			},
			names: []string{"my-server"},
			verify: func(t *testing.T, config map[string]any) {
				// Project-scoped entries are keyed by absolute projectDir.
				// After removal the server should be gone from whichever project map was used.
				projects := config["projects"].(map[string]any)
				for _, proj := range projects {
					p := proj.(map[string]any)
					if servers, ok := p["mcpServers"].(map[string]any); ok {
						if _, ok := servers["my-server"]; ok {
							t.Error("expected my-server to be removed from project scope")
						}
					}
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
			homeDir := t.TempDir()
			t.Setenv("HOME", homeDir)

			projectDir := t.TempDir()

			// For project-scoped test, seed the project entry with a server.
			if tc.scope == projector.ScopeLocal {
				absProject, _ := filepath.Abs(projectDir)
				tc.initialJSON = map[string]any{
					"projects": map[string]any{
						absProject: map[string]any{
							"mcpServers": map[string]any{
								"my-server": map[string]any{"command": "test"},
							},
						},
					},
				}
			}

			configPath := filepath.Join(homeDir, ".claude.json")
			data, _ := json.Marshal(tc.initialJSON)
			if err := os.WriteFile(configPath, data, 0644); err != nil {
				t.Fatal(err)
			}

			c := &claudeCodeProjector{}
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
		})
	}
}
