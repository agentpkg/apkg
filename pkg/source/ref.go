package source

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/agentpkg/agentpkg/pkg/config"
)

// ParseRef parses a user-provided reference into a Source and its config
// representation. Local filesystem paths (starting with ./, ../, or absolute)
// produce a LocalSource. Everything else is treated as a git short-form
// reference: owner/repo/path@ref, mapped to a GitHub HTTPS URL.
func ParseRef(ref string) (Source, config.SkillSource, error) {
	if isLocalPath(ref) {
		src := &LocalSource{Path: ref}
		ss := config.SkillSource{Path: ref}
		return src, ss, nil
	}

	parts := strings.SplitN(ref, "@", 2)
	if len(parts) != 2 || parts[1] == "" {
		return nil, config.SkillSource{}, fmt.Errorf("invalid ref %q: must contain @ref (e.g. owner/repo/path@main)", ref)
	}

	pathPart := parts[0]
	gitRef := parts[1]

	segments := strings.Split(pathPart, "/")
	if len(segments) < 2 {
		return nil, config.SkillSource{}, fmt.Errorf("invalid ref %q: must have at least owner/repo", ref)
	}

	owner := segments[0]
	repo := segments[1]

	var subPath string
	if len(segments) > 2 {
		subPath = strings.Join(segments[2:], "/")
	}

	gitURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
	src := &GitSource{URL: gitURL, Path: subPath, Ref: gitRef}
	ss := config.SkillSource{Git: gitURL, Path: subPath, Ref: gitRef}

	return src, ss, nil
}

// SourceFromSkillConfig converts a config.SkillSource into a Source.
// If Git is set, returns a GitSource; otherwise returns a LocalSource using Path.
func SourceFromSkillConfig(ss config.SkillSource) Source {
	if ss.Git != "" {
		return &GitSource{
			URL:  ss.Git,
			Path: ss.Path,
			Ref:  ss.Ref,
		}
	}

	return &LocalSource{
		Path: ss.Path,
	}
}

func SourceFromMCPConfig(name string, ms config.MCPSource) (Source, error) {
	if ms.Name == "" {
		ms.Name = name
	}

	switch {
	case ms.ManagedStdioMCPConfig != nil && strings.HasPrefix(ms.Package, "npm:"):
		return &NPMSource{Package: strings.TrimPrefix(ms.Package, "npm:"), MCPConfig: ms}, nil
	case ms.ManagedStdioMCPConfig != nil && strings.HasPrefix(ms.Package, "uv:"):
		return &UVSource{Package: strings.TrimPrefix(ms.Package, "uv:"), MCPConfig: ms}, nil
	default:
		return nil, fmt.Errorf("apkg does not currently support installing the MCP config provided")
	}
}

// isLocalPath reports whether ref looks like a local filesystem path.
func isLocalPath(ref string) bool {
	return strings.HasPrefix(ref, "./") || strings.HasPrefix(ref, "../") || filepath.IsAbs(ref)
}
