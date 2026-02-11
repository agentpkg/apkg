package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDevConfig(t *testing.T) {
	tests := map[string]struct {
		globalAgents []string
		localAgents  []string
		flagAgents   []string
		global       bool
		want         []string
	}{
		"project install merges local over global": {
			globalAgents: []string{"claude-code"},
			localAgents:  []string{"claude-code", "gemini"},
			global:       false,
			want:         []string{"claude-code", "gemini"},
		},
		"global install uses only global config": {
			globalAgents: []string{"claude-code"},
			localAgents:  []string{"claude-code", "gemini"},
			global:       true,
			want:         []string{"claude-code"},
		},
		"flag agents override everything for project install": {
			globalAgents: []string{"claude-code"},
			localAgents:  []string{"claude-code", "gemini"},
			flagAgents:   []string{"cursor"},
			global:       false,
			want:         []string{"cursor"},
		},
		"flag agents override everything for global install": {
			globalAgents: []string{"claude-code"},
			localAgents:  []string{"claude-code", "gemini"},
			flagAgents:   []string{"cursor"},
			global:       true,
			want:         []string{"cursor"},
		},
		"no config files returns empty": {
			global: false,
			want:   nil,
		},
		"global install with no global config returns empty": {
			localAgents: []string{"claude-code", "gemini"},
			global:      true,
			want:        nil,
		},
		"project install with only global config uses global": {
			globalAgents: []string{"claude-code"},
			global:       false,
			want:         []string{"claude-code"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()

			globalPath := filepath.Join(dir, "global-config.toml")
			localPath := filepath.Join(dir, "apkg.local.toml")

			if tc.globalAgents != nil {
				writeTestConfig(t, globalPath, tc.globalAgents)
			}
			if tc.localAgents != nil {
				writeTestConfig(t, localPath, tc.localAgents)
			}

			cfg, err := loadDevConfig(tc.flagAgents, tc.global, globalPath, localPath)
			if err != nil {
				t.Fatalf("loadDevConfig() error = %v", err)
			}

			if !slicesEqual(cfg.Agents, tc.want) {
				t.Errorf("Agents = %v, want %v", cfg.Agents, tc.want)
			}
		})
	}
}

func writeTestConfig(t *testing.T, path string, agents []string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating %s: %v", path, err)
	}
	defer f.Close()

	f.WriteString("agents = [")
	for i, a := range agents {
		if i > 0 {
			f.WriteString(", ")
		}
		f.WriteString(`"` + a + `"`)
	}
	f.WriteString("]\n")
}

func slicesEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
