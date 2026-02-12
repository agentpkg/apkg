package projector

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentpkg/agentpkg/pkg/skill"
)

type fakeSkill struct {
	name string
	dir  string
}

var _ skill.Skill = &fakeSkill{}

func (f *fakeSkill) Name() string    { return f.name }
func (f *fakeSkill) Type() string    { return "skill" }
func (f *fakeSkill) Dir() string     { return f.dir }
func (f *fakeSkill) Validate() error { return nil }

func TestSkillProjector_ProjectSkills(t *testing.T) {
	tests := map[string]struct {
		agentDir string
		setup    func(t *testing.T, projectDir string) []skill.Skill
		verify   func(t *testing.T, projectDir, agentDir string)
		wantErr  bool
	}{
		"no packages": {
			agentDir: ".testagent",
			setup: func(t *testing.T, projectDir string) []skill.Skill {
				return nil
			},
			verify: func(t *testing.T, projectDir, agentDir string) {
				skillsDir := filepath.Join(projectDir, agentDir, "skills")
				if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
					t.Error("skills directory was not created")
				}
			},
			wantErr: false,
		},
		"single skill creates symlink": {
			agentDir: ".testagent",
			setup: func(t *testing.T, projectDir string) []skill.Skill {
				skillDir := filepath.Join(t.TempDir(), "my-skill")
				if err := os.Mkdir(skillDir, 0755); err != nil {
					t.Fatal(err)
				}
				return []skill.Skill{&fakeSkill{name: "my-skill", dir: skillDir}}
			},
			verify: func(t *testing.T, projectDir, agentDir string) {
				link := filepath.Join(projectDir, agentDir, "skills", "my-skill")
				info, err := os.Lstat(link)
				if err != nil {
					t.Fatalf("expected symlink at %q: %v", link, err)
				}
				if info.Mode()&os.ModeSymlink == 0 {
					t.Error("expected path to be a symlink")
				}
			},
			wantErr: false,
		},
		"uses correct agent directory": {
			agentDir: ".gemini",
			setup: func(t *testing.T, projectDir string) []skill.Skill {
				skillDir := filepath.Join(t.TempDir(), "my-skill")
				if err := os.Mkdir(skillDir, 0755); err != nil {
					t.Fatal(err)
				}
				return []skill.Skill{&fakeSkill{name: "my-skill", dir: skillDir}}
			},
			verify: func(t *testing.T, projectDir, agentDir string) {
				link := filepath.Join(projectDir, ".gemini", "skills", "my-skill")
				info, err := os.Lstat(link)
				if err != nil {
					t.Fatalf("expected symlink at %q: %v", link, err)
				}
				if info.Mode()&os.ModeSymlink == 0 {
					t.Error("expected path to be a symlink")
				}
				// Ensure it did NOT create a .claude directory
				claudeDir := filepath.Join(projectDir, ".claude")
				if _, err := os.Stat(claudeDir); !os.IsNotExist(err) {
					t.Error("should not have created .claude directory")
				}
			},
			wantErr: false,
		},
		"multiple skills create symlinks": {
			agentDir: ".testagent",
			setup: func(t *testing.T, projectDir string) []skill.Skill {
				var skills []skill.Skill
				for _, name := range []string{"skill-a", "skill-b", "skill-c"} {
					d := filepath.Join(t.TempDir(), name)
					if err := os.Mkdir(d, 0755); err != nil {
						t.Fatal(err)
					}
					skills = append(skills, &fakeSkill{name: name, dir: d})
				}
				return skills
			},
			verify: func(t *testing.T, projectDir, agentDir string) {
				for _, name := range []string{"skill-a", "skill-b", "skill-c"} {
					link := filepath.Join(projectDir, agentDir, "skills", name)
					info, err := os.Lstat(link)
					if err != nil {
						t.Fatalf("expected symlink for %q: %v", name, err)
					}
					if info.Mode()&os.ModeSymlink == 0 {
						t.Errorf("expected %q to be a symlink", name)
					}
				}
			},
			wantErr: false,
		},
		"existing symlink is overwritten": {
			agentDir: ".testagent",
			setup: func(t *testing.T, projectDir string) []skill.Skill {
				skillsDir := filepath.Join(projectDir, ".testagent", "skills")
				if err := os.MkdirAll(skillsDir, 0755); err != nil {
					t.Fatal(err)
				}
				oldTarget := filepath.Join(t.TempDir(), "old")
				if err := os.Mkdir(oldTarget, 0755); err != nil {
					t.Fatal(err)
				}
				link := filepath.Join(skillsDir, "my-skill")
				if err := os.Symlink(oldTarget, link); err != nil {
					t.Fatal(err)
				}

				newTarget := filepath.Join(t.TempDir(), "new")
				if err := os.Mkdir(newTarget, 0755); err != nil {
					t.Fatal(err)
				}
				return []skill.Skill{&fakeSkill{name: "my-skill", dir: newTarget}}
			},
			verify: func(t *testing.T, projectDir, agentDir string) {
				link := filepath.Join(projectDir, agentDir, "skills", "my-skill")
				target, err := os.Readlink(link)
				if err != nil {
					t.Fatalf("expected symlink: %v", err)
				}
				if filepath.Base(filepath.Dir(target)) == "old" {
					t.Error("symlink still points to old target")
				}
			},
			wantErr: false,
		},
		"existing regular file causes error": {
			agentDir: ".testagent",
			setup: func(t *testing.T, projectDir string) []skill.Skill {
				skillsDir := filepath.Join(projectDir, ".testagent", "skills")
				if err := os.MkdirAll(skillsDir, 0755); err != nil {
					t.Fatal(err)
				}
				f := filepath.Join(skillsDir, "my-skill")
				if err := os.WriteFile(f, []byte("not a symlink"), 0644); err != nil {
					t.Fatal(err)
				}

				skillDir := filepath.Join(t.TempDir(), "my-skill")
				if err := os.Mkdir(skillDir, 0755); err != nil {
					t.Fatal(err)
				}
				return []skill.Skill{&fakeSkill{name: "my-skill", dir: skillDir}}
			},
			verify:  func(t *testing.T, projectDir, agentDir string) {},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			projectDir := t.TempDir()
			packages := tc.setup(t, projectDir)

			sp := &SkillProjector{AgentDir: tc.agentDir}
			err := sp.ProjectSkills(ProjectionOpts{ProjectDir: projectDir}, packages)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ProjectSkills() error = %v, wantErr %v", err, tc.wantErr)
			}

			tc.verify(t, projectDir, tc.agentDir)
		})
	}
}
