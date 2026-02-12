package source

import (
	"fmt"
	"testing"

	"github.com/agentpkg/agentpkg/pkg/config"
)

func TestParseRef(t *testing.T) {
	tests := map[string]struct {
		ref       string
		wantErr   bool
		wantLocal bool
		wantGit   string
		wantPath  string
		wantRef   string
	}{
		"git ref with subpath": {
			ref:      "anthropics/skills/skills/pdf@main",
			wantGit:  "https://github.com/anthropics/skills.git",
			wantPath: "skills/pdf",
			wantRef:  "main",
		},
		"git ref without subpath": {
			ref:      "anthropics/skills@v1.0",
			wantGit:  "https://github.com/anthropics/skills.git",
			wantPath: "",
			wantRef:  "v1.0",
		},
		"git ref with deep subpath": {
			ref:      "org/repo/a/b/c/d@feature",
			wantGit:  "https://github.com/org/repo.git",
			wantPath: "a/b/c/d",
			wantRef:  "feature",
		},
		"local relative path ./": {
			ref:       "./my-skills/review",
			wantLocal: true,
			wantPath:  "./my-skills/review",
		},
		"local relative path ../": {
			ref:       "../other/skill",
			wantLocal: true,
			wantPath:  "../other/skill",
		},
		"local absolute path": {
			ref:       "/home/user/skills/pdf",
			wantLocal: true,
			wantPath:  "/home/user/skills/pdf",
		},
		"missing @ref": {
			ref:     "anthropics/skills",
			wantErr: true,
		},
		"empty ref after @": {
			ref:     "anthropics/skills@",
			wantErr: true,
		},
		"only owner, no repo": {
			ref:     "anthropics@main",
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			src, ss, err := ParseRef(tc.ref)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseRef(%q) error = %v, wantErr = %v", tc.ref, err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}

			if tc.wantLocal {
				if _, ok := src.(*LocalSource); !ok {
					t.Fatalf("ParseRef(%q) returned %T, want *LocalSource", tc.ref, src)
				}
				if ss.Path != tc.wantPath {
					t.Errorf("SkillSource.Path = %q, want %q", ss.Path, tc.wantPath)
				}
				if ss.Git != "" {
					t.Errorf("SkillSource.Git = %q, want empty for local", ss.Git)
				}
				return
			}

			gs, ok := src.(*GitSource)
			if !ok {
				t.Fatalf("ParseRef(%q) returned %T, want *GitSource", tc.ref, src)
			}
			if gs.URL != tc.wantGit {
				t.Errorf("GitSource.URL = %q, want %q", gs.URL, tc.wantGit)
			}
			if gs.Path != tc.wantPath {
				t.Errorf("GitSource.Path = %q, want %q", gs.Path, tc.wantPath)
			}
			if gs.Ref != tc.wantRef {
				t.Errorf("GitSource.Ref = %q, want %q", gs.Ref, tc.wantRef)
			}
			if ss.Git != tc.wantGit {
				t.Errorf("SkillSource.Git = %q, want %q", ss.Git, tc.wantGit)
			}
			if ss.Path != tc.wantPath {
				t.Errorf("SkillSource.Path = %q, want %q", ss.Path, tc.wantPath)
			}
			if ss.Ref != tc.wantRef {
				t.Errorf("SkillSource.Ref = %q, want %q", ss.Ref, tc.wantRef)
			}
		})
	}
}

func TestSourceFromSkillConfig(t *testing.T) {
	tests := map[string]struct {
		input      config.SkillSource
		wantType   string
		wantGitURL string
		wantPath   string
	}{
		"git source": {
			input: config.SkillSource{
				Git:  "https://github.com/anthropics/skills.git",
				Path: "skills/pdf",
				Ref:  "main",
			},
			wantType:   "git",
			wantGitURL: "https://github.com/anthropics/skills.git",
			wantPath:   "skills/pdf",
		},
		"local source": {
			input: config.SkillSource{
				Path: "./my-skills/review",
			},
			wantType: "local",
			wantPath: "./my-skills/review",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			src := SourceFromSkillConfig(tc.input)

			switch tc.wantType {
			case "git":
				gs, ok := src.(*GitSource)
				if !ok {
					t.Fatalf("SourceFromConfig() returned %T, want *GitSource", src)
				}
				if gs.URL != tc.wantGitURL {
					t.Errorf("URL = %q, want %q", gs.URL, tc.wantGitURL)
				}
				if gs.Path != tc.wantPath {
					t.Errorf("Path = %q, want %q", gs.Path, tc.wantPath)
				}
			case "local":
				ls, ok := src.(*LocalSource)
				if !ok {
					t.Fatalf("SourceFromConfig() returned %T, want *LocalSource", src)
				}
				if ls.Path != tc.wantPath {
					t.Errorf("Path = %q, want %q", ls.Path, tc.wantPath)
				}
			}
		})
	}
}

func TestSourceFromMCPConfig(t *testing.T) {
	tests := map[string]struct {
		name      string
		ms        config.MCPSource
		wantType  string
		wantErr   bool
	}{
		"npm managed stdio": {
			name: "npm-server",
			ms: config.MCPSource{
				Transport:            "stdio",
				ManagedStdioMCPConfig: &config.ManagedStdioMCPConfig{Package: "npm:some-pkg@1.0.0"},
			},
			wantType: "*source.NPMSource",
		},
		"uv managed stdio": {
			name: "uv-server",
			ms: config.MCPSource{
				Transport:            "stdio",
				ManagedStdioMCPConfig: &config.ManagedStdioMCPConfig{Package: "uv:some-pkg==1.0.0"},
			},
			wantType: "*source.UVSource",
		},
		"unmanaged stdio": {
			name: "local-server",
			ms: config.MCPSource{
				Transport:              "stdio",
				UnmanagedStdioMCPConfig: &config.UnmanagedStdioMCPConfig{Command: "/usr/bin/echo"},
			},
			wantType: "*source.StaticSource",
		},
		"external http": {
			name: "remote-server",
			ms: config.MCPSource{
				Transport:            "http",
				ExternalHttpMCPConfig: &config.ExternalHttpMCPConfig{URL: "https://example.com/mcp"},
			},
			wantType: "*source.StaticSource",
		},
		"unsupported config": {
			name:    "bad-server",
			ms:      config.MCPSource{Transport: "stdio"},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			src, err := SourceFromMCPConfig(tc.name, tc.ms)
			if (err != nil) != tc.wantErr {
				t.Fatalf("SourceFromMCPConfig() error = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}

			gotType := fmt.Sprintf("%T", src)
			if gotType != tc.wantType {
				t.Errorf("SourceFromMCPConfig() returned %s, want %s", gotType, tc.wantType)
			}
		})
	}
}

func TestIsLocalPath(t *testing.T) {
	tests := map[string]struct {
		input string
		want  bool
	}{
		"dot slash": {
			input: "./foo",
			want:  true,
		},
		"dot dot slash": {
			input: "../bar",
			want:  true,
		},
		"absolute path": {
			input: "/home/user/skill",
			want:  true,
		},
		"git ref": {
			input: "owner/repo@main",
			want:  false,
		},
		"bare name": {
			input: "my-skill",
			want:  false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := isLocalPath(tc.input)
			if got != tc.want {
				t.Errorf("isLocalPath(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
