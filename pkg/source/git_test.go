package source

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentpkg/agentpkg/pkg/store"
)

// requireGit skips the test if git is not available.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
}

// setupBareRepo creates a bare git repo with a single commit containing
// files at skills/pdf/manifest.toml and README.md. It creates a lightweight
// tag "v1.0" and an annotated tag "v2.0" pointing at the same commit.
// Returns the bare repo path (usable as a git URL) and the commit hash.
func setupBareRepo(t *testing.T) (repoURL string, commit string) {
	t.Helper()

	workDir := filepath.Join(t.TempDir(), "work")

	for _, args := range [][]string{
		{"init", "--initial-branch=main", workDir},
		{"-C", workDir, "config", "user.email", "test@test.com"},
		{"-C", workDir, "config", "user.name", "Test"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	os.MkdirAll(filepath.Join(workDir, "skills", "pdf"), 0o755)
	os.WriteFile(filepath.Join(workDir, "skills", "pdf", "manifest.toml"), []byte("[skill]\nname = \"pdf\"\n"), 0o644)
	os.WriteFile(filepath.Join(workDir, "README.md"), []byte("# test\n"), 0o644)

	for _, args := range [][]string{
		{"-C", workDir, "add", "."},
		{"-C", workDir, "commit", "-m", "initial commit"},
		{"-C", workDir, "tag", "v1.0"},
		{"-C", workDir, "tag", "-a", "v2.0", "-m", "version 2.0"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	out, err := exec.Command("git", "-C", workDir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	commit = strings.TrimSpace(string(out))

	bareDir := filepath.Join(t.TempDir(), "repo.git")
	if out, err := exec.Command("git", "clone", "--bare", workDir, bareDir).CombinedOutput(); err != nil {
		t.Fatalf("git clone --bare: %v\n%s", err, out)
	}

	return bareDir, commit
}

func TestIsCommitHash(t *testing.T) {
	tests := map[string]struct {
		input string
		want  bool
	}{
		"valid lowercase": {
			input: "0123456789abcdef0123456789abcdef01234567",
			want:  true,
		},
		"valid uppercase": {
			input: "0123456789ABCDEF0123456789ABCDEF01234567",
			want:  true,
		},
		"valid mixed case": {
			input: "0123456789aBcDeF0123456789AbCdEf01234567",
			want:  true,
		},
		"too short": {
			input: "0123456789abcdef",
			want:  false,
		},
		"too long": {
			input: "0123456789abcdef0123456789abcdef012345678",
			want:  false,
		},
		"empty string": {
			input: "",
			want:  false,
		},
		"non-hex characters": {
			input: "xyz123def456abc123def456abc123def456abc123",
			want:  false,
		},
		"branch name": {
			input: "main",
			want:  false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := isCommitHash(tc.input)
			if got != tc.want {
				t.Errorf("isCommitHash(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestIsShortCommitHash(t *testing.T) {
	tests := map[string]struct {
		input string
		want  bool
	}{
		"7 hex chars": {
			input: "abcdef0",
			want:  true,
		},
		"12 hex chars": {
			input: "0123456789ab",
			want:  true,
		},
		"39 hex chars": {
			input: "0123456789abcdef0123456789abcdef0123456",
			want:  true,
		},
		"40 hex chars is full hash": {
			input: "0123456789abcdef0123456789abcdef01234567",
			want:  false,
		},
		"6 chars too short": {
			input: "abcdef",
			want:  false,
		},
		"non-hex": {
			input: "ghijklm",
			want:  false,
		},
		"branch name": {
			input: "main",
			want:  false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := isShortCommitHash(tc.input)
			if got != tc.want {
				t.Errorf("isShortCommitHash(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseGitURL(t *testing.T) {
	tests := map[string]struct {
		rawURL   string
		wantHost string
		wantPath string
		wantErr  bool
	}{
		"https with .git suffix": {
			rawURL:   "https://github.com/anthropics/skills.git",
			wantHost: "github.com",
			wantPath: "anthropics/skills",
		},
		"https without .git suffix": {
			rawURL:   "https://github.com/anthropics/skills",
			wantHost: "github.com",
			wantPath: "anthropics/skills",
		},
		"ssh shorthand with .git suffix": {
			rawURL:   "git@github.com:anthropics/skills.git",
			wantHost: "github.com",
			wantPath: "anthropics/skills",
		},
		"ssh shorthand without .git suffix": {
			rawURL:   "git@github.com:anthropics/skills",
			wantHost: "github.com",
			wantPath: "anthropics/skills",
		},
		"ssh protocol url": {
			rawURL:   "ssh://git@github.com/anthropics/skills.git",
			wantHost: "github.com",
			wantPath: "anthropics/skills",
		},
		"nested path": {
			rawURL:   "https://gitlab.com/org/group/subgroup/repo.git",
			wantHost: "gitlab.com",
			wantPath: "org/group/subgroup/repo",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			host, path, err := parseGitURL(tc.rawURL)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseGitURL(%q) error = %v, wantErr = %v", tc.rawURL, err, tc.wantErr)
			}
			if host != tc.wantHost {
				t.Errorf("parseGitURL(%q) host = %q, want %q", tc.rawURL, host, tc.wantHost)
			}
			if path != tc.wantPath {
				t.Errorf("parseGitURL(%q) path = %q, want %q", tc.rawURL, path, tc.wantPath)
			}
		})
	}
}

func TestRepoSegments(t *testing.T) {
	tests := map[string]struct {
		source GitSource
		commit string
		want   []string
	}{
		"https url": {
			source: GitSource{URL: "https://github.com/anthropics/skills.git"},
			commit: "abc123",
			want:   []string{"repos", "github.com", "anthropics", "skills", "abc123"},
		},
		"ssh shorthand": {
			source: GitSource{URL: "git@github.com:anthropics/skills.git"},
			commit: "def456",
			want:   []string{"repos", "github.com", "anthropics", "skills", "def456"},
		},
		"nested path": {
			source: GitSource{URL: "https://gitlab.com/org/group/repo.git"},
			commit: "aaa111",
			want:   []string{"repos", "gitlab.com", "org", "group", "repo", "aaa111"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := tc.source.repoSegments(tc.commit)
			if err != nil {
				t.Fatalf("repoSegments() error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("repoSegments() = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("repoSegments()[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestResolveRef(t *testing.T) {
	requireGit(t)
	repoURL, wantCommit := setupBareRepo(t)

	tests := map[string]struct {
		ref string
	}{
		"branch": {
			ref: "main",
		},
		"lightweight tag": {
			ref: "v1.0",
		},
		"annotated tag": {
			ref: "v2.0",
		},
		"commit hash": {
			ref: wantCommit,
		},
		"short commit hash": {
			ref: wantCommit[:12],
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			g := &GitSource{URL: repoURL, Ref: tc.ref}
			got, err := g.resolveRef(context.Background())
			if err != nil {
				t.Fatalf("resolveRef() error: %v", err)
			}
			if got != wantCommit {
				t.Errorf("resolveRef() = %q, want %q", got, wantCommit)
			}
		})
	}
}

func TestResolveRefNotFound(t *testing.T) {
	requireGit(t)
	repoURL, _ := setupBareRepo(t)

	g := &GitSource{URL: repoURL, Ref: "nonexistent"}
	_, err := g.resolveRef(context.Background())
	if err == nil {
		t.Fatal("expected error for nonexistent ref, got nil")
	}
}

func TestFetch(t *testing.T) {
	requireGit(t)
	repoURL, wantCommit := setupBareRepo(t)

	tests := map[string]struct {
		ref  string
		path string
	}{
		"branch ref with subpath": {
			ref:  "main",
			path: "skills/pdf",
		},
		"lightweight tag with subpath": {
			ref:  "v1.0",
			path: "skills/pdf",
		},
		"annotated tag with subpath": {
			ref:  "v2.0",
			path: "skills/pdf",
		},
		"commit hash with subpath": {
			ref:  wantCommit,
			path: "skills/pdf",
		},
		"short commit hash with subpath": {
			ref:  wantCommit[:12],
			path: "skills/pdf",
		},
		"no subpath": {
			ref:  "main",
			path: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := store.New(t.TempDir())
			g := &GitSource{URL: repoURL, Ref: tc.ref, Path: tc.path}

			result, err := g.Fetch(context.Background(), s)
			if err != nil {
				t.Fatalf("Fetch() error: %v", err)
			}

			if result.Commit != wantCommit {
				t.Errorf("Commit = %q, want %q", result.Commit, wantCommit)
			}
			if result.Ref != tc.ref {
				t.Errorf("Ref = %q, want %q", result.Ref, tc.ref)
			}
			if !strings.HasPrefix(result.Integrity, "sha256:") {
				t.Errorf("Integrity = %q, want sha256: prefix", result.Integrity)
			}

			info, err := os.Stat(result.Dir)
			if err != nil {
				t.Fatalf("Dir %q does not exist: %v", result.Dir, err)
			}
			if !info.IsDir() {
				t.Fatalf("Dir %q is not a directory", result.Dir)
			}

			if tc.path == "skills/pdf" {
				manifest := filepath.Join(result.Dir, "manifest.toml")
				if _, err := os.Stat(manifest); err != nil {
					t.Errorf("expected manifest.toml in %q: %v", result.Dir, err)
				}
			}
		})
	}
}

func TestFetchCached(t *testing.T) {
	requireGit(t)
	repoURL, _ := setupBareRepo(t)

	s := store.New(t.TempDir())
	g := &GitSource{URL: repoURL, Ref: "v1.0", Path: "skills/pdf"}

	first, err := g.Fetch(context.Background(), s)
	if err != nil {
		t.Fatalf("first Fetch() error: %v", err)
	}

	second, err := g.Fetch(context.Background(), s)
	if err != nil {
		t.Fatalf("second Fetch() error: %v", err)
	}

	if first.Dir != second.Dir {
		t.Errorf("Dir mismatch: %q vs %q", first.Dir, second.Dir)
	}
	if first.Commit != second.Commit {
		t.Errorf("Commit mismatch: %q vs %q", first.Commit, second.Commit)
	}
	if first.Integrity != second.Integrity {
		t.Errorf("Integrity mismatch: %q vs %q", first.Integrity, second.Integrity)
	}
}

func TestFetchContextCanceled(t *testing.T) {
	requireGit(t)
	repoURL, _ := setupBareRepo(t)

	s := store.New(t.TempDir())
	g := &GitSource{URL: repoURL, Ref: "main", Path: "skills/pdf"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := g.Fetch(ctx, s)
	if err == nil {
		t.Fatal("expected error with canceled context, got nil")
	}
}
