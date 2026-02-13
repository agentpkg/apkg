package installer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentpkg/agentpkg/pkg/config"
	"github.com/agentpkg/agentpkg/pkg/source"
	"github.com/agentpkg/agentpkg/pkg/store"
)

// writeSkill creates a minimal SKILL.md in dir with the given name.
func writeSkill(t *testing.T, dir, name string) {
	t.Helper()
	os.MkdirAll(dir, 0o755)
	content := "---\nname: " + name + "\ndescription: test skill\n---\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("writing SKILL.md: %v", err)
	}
}

func TestInstallAll(t *testing.T) {
	tests := map[string]struct {
		skills    map[string]config.SkillSource
		existing  *config.LockFile
		wantCount int
		wantErr   bool
	}{
		"empty config": {
			skills:    map[string]config.SkillSource{},
			wantCount: 0,
		},
		"single local skill": {
			skills: func() map[string]config.SkillSource {
				dir := t.TempDir()
				writeSkill(t, dir, "my-skill")
				return map[string]config.SkillSource{
					"my-skill": {Path: dir},
				}
			}(),
			wantCount: 1,
		},
		"multiple local skills": {
			skills: func() map[string]config.SkillSource {
				dir1 := t.TempDir()
				writeSkill(t, dir1, "skill-a")
				dir2 := t.TempDir()
				writeSkill(t, dir2, "skill-b")
				return map[string]config.SkillSource{
					"skill-a": {Path: dir1},
					"skill-b": {Path: dir2},
				}
			}(),
			wantCount: 2,
		},
		"missing skill directory": {
			skills: map[string]config.SkillSource{
				"missing": {Path: "/nonexistent/path"},
			},
			wantErr: true,
		},
		"nil existing lockfile": {
			skills: func() map[string]config.SkillSource {
				dir := t.TempDir()
				writeSkill(t, dir, "my-skill")
				return map[string]config.SkillSource{
					"my-skill": {Path: dir},
				}
			}(),
			existing:  nil,
			wantCount: 1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			inst := &Installer{
				Store:      store.New(t.TempDir()),
				ProjectDir: t.TempDir(),
				Agents:     []string{},
			}

			cfg := &config.Config{
				Project: config.ProjectConfig{Name: "test"},
				Skills:  tc.skills,
			}

			lf, err := inst.InstallAll(context.Background(), cfg, tc.existing)
			if (err != nil) != tc.wantErr {
				t.Fatalf("InstallAll() error = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}

			if len(lf.Skills) != tc.wantCount {
				t.Errorf("lockfile has %d skills, want %d", len(lf.Skills), tc.wantCount)
			}
			if lf.Version != 1 {
				t.Errorf("lockfile version = %d, want 1", lf.Version)
			}
		})
	}
}

func TestInstallSkill(t *testing.T) {
	tests := map[string]struct {
		setupDir func(t *testing.T) string
		wantName string
		wantErr  bool
	}{
		"valid local skill": {
			setupDir: func(t *testing.T) string {
				dir := t.TempDir()
				writeSkill(t, dir, "test-skill")
				return dir
			},
			wantName: "test-skill",
		},
		"missing SKILL.md": {
			setupDir: func(t *testing.T) string {
				return t.TempDir()
			},
			wantErr: true,
		},
		"invalid skill missing description": {
			setupDir: func(t *testing.T) string {
				dir := t.TempDir()
				content := "---\nname: bad\n---\n# bad\n"
				os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644)
				return dir
			},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dir := tc.setupDir(t)

			inst := &Installer{
				Store:      store.New(t.TempDir()),
				ProjectDir: t.TempDir(),
				Agents:     []string{},
			}

			src := &source.LocalSource{Path: dir}

			sk, resolved, err := inst.InstallSkill(context.Background(), src)
			if (err != nil) != tc.wantErr {
				t.Fatalf("InstallSkill() error = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}

			if sk.Name() != tc.wantName {
				t.Errorf("skill name = %q, want %q", sk.Name(), tc.wantName)
			}
			if resolved.Dir == "" {
				t.Error("resolved dir is empty")
			}
		})
	}
}

func TestBuildLockIndex(t *testing.T) {
	tests := map[string]struct {
		lockfile *config.LockFile
		wantLen  int
	}{
		"nil lockfile": {
			lockfile: nil,
			wantLen:  0,
		},
		"empty lockfile": {
			lockfile: &config.LockFile{Version: 1},
			wantLen:  0,
		},
		"lockfile with entries": {
			lockfile: &config.LockFile{
				Version: 1,
				Skills: []config.SkillLockEntry{
					{Git: "https://github.com/a/b.git", Path: "skills/c", Commit: "abc"},
					{Path: "./local"},
				},
			},
			wantLen: 2,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			idx := buildLockIndex(tc.lockfile)
			if tc.wantLen == 0 {
				if idx != nil && len(idx) != 0 {
					t.Errorf("buildLockIndex() len = %d, want 0", len(idx))
				}
				return
			}
			if len(idx) != tc.wantLen {
				t.Errorf("buildLockIndex() len = %d, want %d", len(idx), tc.wantLen)
			}
		})
	}
}

func TestRemoveSkill(t *testing.T) {
	tests := map[string]struct {
		setup   func(t *testing.T, projectDir string) string // returns skill name
		wantErr bool
	}{
		"removes projected symlink": {
			setup: func(t *testing.T, projectDir string) string {
				// Create a skill dir and project it first.
				skillDir := filepath.Join(t.TempDir(), "my-skill")
				os.MkdirAll(skillDir, 0755)
				writeSkill(t, skillDir, "my-skill")

				inst := &Installer{
					Store:      store.New(t.TempDir()),
					ProjectDir: projectDir,
					Agents:     []string{},
				}
				_, _, err := inst.InstallSkill(context.Background(), &source.LocalSource{Path: skillDir})
				if err != nil {
					t.Fatalf("InstallSkill setup: %v", err)
				}
				return "my-skill"
			},
		},
		"no-op when no agents": {
			setup: func(t *testing.T, projectDir string) string {
				return "nonexistent-skill"
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			projectDir := t.TempDir()
			skillName := tc.setup(t, projectDir)

			inst := &Installer{
				Store:      store.New(t.TempDir()),
				ProjectDir: projectDir,
				Agents:     []string{},
			}

			err := inst.RemoveSkill(skillName)
			if (err != nil) != tc.wantErr {
				t.Fatalf("RemoveSkill() error = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestRemoveMCP(t *testing.T) {
	tests := map[string]struct {
		name    string
		wantErr bool
	}{
		"no-op when no agents": {
			name: "my-server",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			inst := &Installer{
				Store:      store.New(t.TempDir()),
				ProjectDir: t.TempDir(),
				Agents:     []string{},
			}

			err := inst.RemoveMCP(tc.name)
			if (err != nil) != tc.wantErr {
				t.Fatalf("RemoveMCP() error = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestLockKey(t *testing.T) {
	tests := map[string]struct {
		input config.SkillSource
		want  string
	}{
		"git source": {
			input: config.SkillSource{Git: "https://github.com/a/b.git", Path: "skills/c"},
			want:  "https://github.com/a/b.git|skills/c",
		},
		"local source": {
			input: config.SkillSource{Path: "./my-skill"},
			want:  "./my-skill",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := lockKey(tc.input)
			if got != tc.want {
				t.Errorf("lockKey() = %q, want %q", got, tc.want)
			}
		})
	}
}
