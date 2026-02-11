package skill

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	_, f, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get caller info")
	}
	return filepath.Join(filepath.Dir(f), "testdata")
}

func TestLoad(t *testing.T) {
	tests := map[string]struct {
		dir      string
		wantName string
		wantErr  bool
	}{
		"valid basic skill": {
			dir:      "valid-basic",
			wantName: "my-skill",
		},
		"valid skill with all fields": {
			dir:      "valid-all-fields",
			wantName: "full-skill",
		},
		"valid skill with metadata": {
			dir:      "valid-metadata",
			wantName: "meta-skill",
		},
		"no front matter delimiters": {
			dir:     "no-frontmatter",
			wantErr: true,
		},
		"empty file": {
			dir:     "empty-file",
			wantErr: true,
		},
		"missing SKILL.md file": {
			dir:     "no-skill-file",
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(testdataDir(t), tc.dir)
			s, err := Load(dir)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Load() error = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if s.Name() != tc.wantName {
				t.Errorf("Name() = %q, want %q", s.Name(), tc.wantName)
			}
		})
	}
}

func TestSkillAccessors(t *testing.T) {
	dir := filepath.Join(testdataDir(t), "valid-basic")
	s, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got := s.Name(); got != "my-skill" {
		t.Errorf("Name() = %q, want %q", got, "my-skill")
	}
	if got := s.Type(); got != TypeSkill {
		t.Errorf("Type() = %q, want %q", got, TypeSkill)
	}
	if got := s.Dir(); got != dir {
		t.Errorf("Dir() = %q, want %q", got, dir)
	}
}

func TestValidate(t *testing.T) {
	tests := map[string]struct {
		skill      skill
		wantErr    bool
		wantErrMsg string
	}{
		"valid skill": {
			skill: skill{
				SkillName:   "my-skill",
				Description: "a valid description",
			},
		},
		"valid name single char": {
			skill: skill{
				SkillName:   "a",
				Description: "desc",
			},
		},
		"valid name max length": {
			skill: skill{
				SkillName:   "a" + strings.Repeat("-a", 31),
				Description: "desc",
			},
		},
		"invalid name with uppercase": {
			skill: skill{
				SkillName:   "My-Skill",
				Description: "desc",
			},
			wantErr:    true,
			wantErrMsg: "skill name must be max 64 characters",
		},
		"invalid name starts with hyphen": {
			skill: skill{
				SkillName:   "-my-skill",
				Description: "desc",
			},
			wantErr:    true,
			wantErrMsg: "skill name must be max 64 characters",
		},
		"invalid name ends with hyphen": {
			skill: skill{
				SkillName:   "my-skill-",
				Description: "desc",
			},
			wantErr:    true,
			wantErrMsg: "skill name must be max 64 characters",
		},
		"invalid name with underscore": {
			skill: skill{
				SkillName:   "my_skill",
				Description: "desc",
			},
			wantErr:    true,
			wantErrMsg: "skill name must be max 64 characters",
		},
		"empty name": {
			skill: skill{
				SkillName:   "",
				Description: "desc",
			},
			wantErr:    true,
			wantErrMsg: "skill name must be max 64 characters",
		},
		"empty description": {
			skill: skill{
				SkillName:   "my-skill",
				Description: "",
			},
			wantErr:    true,
			wantErrMsg: "skill description must be provided",
		},
		"description too long": {
			skill: skill{
				SkillName:   "my-skill",
				Description: strings.Repeat("a", 1025),
			},
			wantErr:    true,
			wantErrMsg: "skill description must be max 1024 characters",
		},
		"description exactly at limit": {
			skill: skill{
				SkillName:   "my-skill",
				Description: strings.Repeat("a", 1024),
			},
		},
		"compatability too long": {
			skill: skill{
				SkillName:     "my-skill",
				Description:   "desc",
				Compatability: strings.Repeat("a", 501),
			},
			wantErr:    true,
			wantErrMsg: "compatability must be max 500 characters",
		},
		"compatability exactly at limit": {
			skill: skill{
				SkillName:     "my-skill",
				Description:   "desc",
				Compatability: strings.Repeat("a", 500),
			},
		},
		"multiple validation errors": {
			skill: skill{
				SkillName:     "",
				Description:   "",
				Compatability: strings.Repeat("a", 501),
			},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := tc.skill.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErrMsg != "" && err != nil {
				if !strings.Contains(err.Error(), tc.wantErrMsg) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tc.wantErrMsg)
				}
			}
		})
	}
}
