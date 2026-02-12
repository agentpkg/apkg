package source

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/agentpkg/agentpkg/pkg/config"
	"github.com/agentpkg/agentpkg/pkg/store"
	"github.com/pelletier/go-toml/v2"
)

const (
	mcpFileName  = "mcp.toml"
	mcpFilePerms = 0o644
)

type NPMSource struct {
	Package   string
	MCPConfig config.MCPSource
}

var _ Source = &NPMSource{}

func (s *NPMSource) Fetch(ctx context.Context, store store.Store) (*ResolvedSource, error) {
	version, err := s.resolveConcreteVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get concrete version for npm package: %w", err)
	}

	segs := s.getStoreSegments(version)

	cached, err := store.Exists(segs...)
	if err != nil {
		return nil, fmt.Errorf("failed to check cached npm package: %w", err)
	}

	if !cached {
		// create dirs and install package
		store.EnsureDir(segs...)
		path := store.Path(segs...)

		if err := s.install(ctx, path, version); err != nil {
			store.Remove(segs...)
			return nil, fmt.Errorf("failed to install npm package %s@%s: %w", s.packageName(), version, err)
		}

		if err := s.writeMCPConfig(store, segs); err != nil {
			store.Remove(segs...)
			return nil, fmt.Errorf("writing mcp config: %w", err)
		}
	}

	integrity, err := store.HashDir(segs...)
	if err != nil {
		return nil, fmt.Errorf("failed to compute integrity hash: %w", err)
	}

	return &ResolvedSource{
		Dir:       store.Path(segs...),
		Integrity: integrity,
	}, nil
}

func (s *NPMSource) resolveConcreteVersion(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "npm", "view", s.Package, "version", "--json")
	out, err := cmd.Output()
	if err != nil {
		return "", execError(err)
	}

	// output is either a string or an array of strings - check both

	var version string
	if err := json.Unmarshal(out, &version); err == nil {
		return version, nil
	}

	var versions []string
	if err := json.Unmarshal(out, &versions); err != nil {
		return "", fmt.Errorf("failed to parse 'npm view %s version --json' output: %w", s.Package, err)
	}

	if len(versions) == 0 {
		return "", fmt.Errorf("no versions found for %s", s.Package)
	}

	// pick first returned version (note: this is an arbitraty choice)
	return versions[0], nil
}

func (s *NPMSource) getStoreSegments(resolvedVersion string) []string {
	packageParts := strings.Split(s.packageName(), "/")

	segs := make([]string, 0, 2+len(packageParts))
	segs = append(segs, "npm")
	segs = append(segs, packageParts...)
	segs = append(segs, resolvedVersion)

	return segs
}

func (s *NPMSource) packageName() string {
	packageName := s.Package
	// if idx == -1, no version tag, if idx == 0, then there is a scoped package, and no version tag (i.e. @modencontextprotocol/inspector)
	if idx := strings.LastIndex(packageName, "@"); idx > 0 {
		packageName = packageName[:idx]
	}

	return packageName
}

func (s *NPMSource) install(ctx context.Context, dest string, version string) error {
	pkg := fmt.Sprintf("%s@%s", s.packageName(), version)

	cmd := exec.CommandContext(ctx, "npm", "install", "--prefix", dest, pkg)
	if _, err := cmd.Output(); err != nil {
		return execError(err)
	}

	return nil
}

func (s *NPMSource) writeMCPConfig(store store.Store, segs []string) error {
	data, err := toml.Marshal(s.MCPConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal mcp config: %w", err)
	}

	mcpSegs := make([]string, len(segs)+1)
	copy(mcpSegs, segs)
	mcpSegs[len(mcpSegs)-1] = mcpFileName

	return store.WriteFile(data, mcpFilePerms, mcpSegs...)
}
