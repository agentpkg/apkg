package source

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strings"

	"github.com/agentpkg/agentpkg/pkg/store"
)

type GitSource struct {
	URL  string
	Path string
	Ref  string
}

var _ Source = &GitSource{}

func (g *GitSource) Fetch(ctx context.Context, s store.Store) (*ResolvedSource, error) {
	// 1. Resolve the ref to a commit hash.
	commit, err := g.resolveRef(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolving ref %q: %w", g.Ref, err)
	}

	// 2. Check if this repo@commit is already cached.
	segs, err := g.repoSegments(commit)
	if err != nil {
		return nil, err
	}

	cached, err := s.Exists(segs...)
	if err != nil {
		return nil, fmt.Errorf("checking cache: %w", err)
	}

	if !cached {
		// 3. Create parent directories for the clone destination.
		s.EnsureDir(segs[:len(segs)-1]...)

		// 4-5. Clone the repository into the store path.
		dest := s.Path(segs...)
		if err := g.clone(ctx, dest, commit); err != nil {
			s.Remove(segs...)
			return nil, fmt.Errorf("cloning %s: %w", g.URL, err)
		}
	}

	// 6. Compute integrity hash over the content subdirectory.
	contentSegs := make([]string, len(segs))
	copy(contentSegs, segs)
	if g.Path != "" {
		contentSegs = append(contentSegs, strings.Split(g.Path, "/")...)
	}

	integrity, err := s.HashDir(contentSegs...)
	if err != nil {
		return nil, fmt.Errorf("computing integrity hash: %w", err)
	}

	// 7. Return the resolved source pointing to the subpath within the clone.
	return &ResolvedSource{
		Dir:       s.Path(contentSegs...),
		Commit:    commit,
		Ref:       g.Ref,
		Integrity: integrity,
	}, nil
}

// resolveRef resolves g.Ref to a full 40-char commit hash.
// Full commit hashes are returned as-is. Short commit hashes are resolved
// via git fetch + rev-parse. Branch and tag names are resolved via ls-remote.
func (g *GitSource) resolveRef(ctx context.Context) (string, error) {
	if isCommitHash(g.Ref) {
		return g.Ref, nil
	}

	if isShortCommitHash(g.Ref) {
		return g.resolveShortHash(ctx)
	}

	cmd := exec.CommandContext(ctx, "git", "ls-remote", g.URL, g.Ref, g.Ref+"^{}")
	out, err := cmd.Output()
	if err != nil {
		return "", execError(err)
	}

	var commit string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		commit = fields[0]
		// For annotated tags, prefer the dereferenced entry (^{})
		// which points to the underlying commit.
		if strings.HasSuffix(fields[1], "^{}") {
			return fields[0], nil
		}
	}

	if commit == "" {
		return "", fmt.Errorf("ref %q not found in %s", g.Ref, g.URL)
	}
	return commit, nil
}

// resolveShortHash expands a short commit hash to the full 40-char hash
// by listing all refs and prefix-matching their commit hashes.
func (g *GitSource) resolveShortHash(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-remote", g.URL)
	out, err := cmd.Output()
	if err != nil {
		return "", execError(err)
	}

	prefix := strings.ToLower(g.Ref)
	var match string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		hash := strings.ToLower(fields[0])
		if !strings.HasPrefix(hash, prefix) {
			continue
		}
		if match != "" && match != hash {
			return "", fmt.Errorf("short hash %q is ambiguous in %s", g.Ref, g.URL)
		}
		match = hash
	}

	if match == "" {
		return "", fmt.Errorf("short hash %q not found in %s", g.Ref, g.URL)
	}
	return match, nil
}

// clone performs a shallow clone of the repository into dest.
// Uses --branch for branch/tag refs, and init+fetch for commit hashes.
// commit is the full resolved hash used for the fetch-by-SHA path.
func (g *GitSource) clone(ctx context.Context, dest string, commit string) error {
	if isHexString(g.Ref) {
		return g.cloneCommit(ctx, dest, commit)
	}
	return g.cloneBranch(ctx, dest)
}

func (g *GitSource) cloneBranch(ctx context.Context, dest string) error {
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--branch", g.Ref, g.URL, dest)
	if _, err := cmd.Output(); err != nil {
		return execError(err)
	}
	return nil
}

// cloneCommit fetches a single commit by SHA. Requires the server to support
// uploadpack.allowReachableSHA1InWant (GitHub, GitLab, and Bitbucket do).
func (g *GitSource) cloneCommit(ctx context.Context, dest string, commit string) error {
	for _, args := range [][]string{
		{"init", dest},
		{"-C", dest, "remote", "add", "origin", g.URL},
		{"-C", dest, "fetch", "--depth", "1", "origin", commit},
		{"-C", dest, "checkout", "FETCH_HEAD"},
	} {
		cmd := exec.CommandContext(ctx, "git", args...)
		if _, err := cmd.Output(); err != nil {
			return execError(err)
		}
	}
	return nil
}

// repoSegments returns the store path segments for caching this repo at a given commit.
// e.g. "https://github.com/anthropics/skills.git" at commit "abc123..." →
//
//	["repos", "github.com", "anthropics", "skills", "abc123..."]
func (g *GitSource) repoSegments(commit string) ([]string, error) {
	host, repoPath, err := parseGitURL(g.URL)
	if err != nil {
		return nil, fmt.Errorf("parsing git URL: %w", err)
	}
	segs := []string{"repos", host}
	segs = append(segs, strings.Split(repoPath, "/")...)
	segs = append(segs, commit)
	return segs, nil
}

// parseGitURL extracts the host and repository path from a git URL.
// Supports HTTPS URLs and SSH shorthand (git@host:owner/repo.git).
func parseGitURL(rawURL string) (host, repoPath string, err error) {
	// SSH shorthand: git@github.com:owner/repo.git
	if idx := strings.Index(rawURL, ":"); idx > 0 && !strings.Contains(rawURL[:idx], "/") && !strings.Contains(rawURL, "://") {
		host = rawURL[:idx]
		if at := strings.Index(host, "@"); at >= 0 {
			host = host[at+1:]
		}
		repoPath = strings.TrimSuffix(rawURL[idx+1:], ".git")
		return host, repoPath, nil
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", err
	}
	repoPath = strings.TrimPrefix(u.Path, "/")
	repoPath = strings.TrimSuffix(repoPath, ".git")
	return u.Host, repoPath, nil
}

// isCommitHash reports whether s is a full 40-character hex SHA-1 hash.
func isCommitHash(s string) bool {
	return len(s) == 40 && isHexString(s)
}

// isShortCommitHash reports whether s looks like an abbreviated commit hash (7-39 hex chars).
func isShortCommitHash(s string) bool {
	return len(s) >= 7 && len(s) < 40 && isHexString(s)
}

// isHexString reports whether s is non-empty and contains only hexadecimal characters.
func isHexString(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func execError(err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
	}
	return err
}
