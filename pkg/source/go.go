package source

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/agentpkg/agentpkg/pkg/config"
	"github.com/agentpkg/agentpkg/pkg/store"
	"github.com/pelletier/go-toml/v2"
)

type GoSource struct {
	Package   string
	MCPConfig config.MCPSource
}

var _ Source = &GoSource{}

func (s *GoSource) Fetch(ctx context.Context, store store.Store) (*ResolvedSource, error) {
	version, err := s.resolveConcreteVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get concrete version for go module: %w", err)
	}

	segs := s.getStoreSegments(version)

	cached, err := store.Exists(segs...)
	if err != nil {
		return nil, fmt.Errorf("failed to check cached go module: %w", err)
	}

	if !cached {
		store.EnsureDir(segs...)
		path := store.Path(segs...)

		if err := s.install(ctx, path, version); err != nil {
			store.Remove(segs...)
			return nil, fmt.Errorf("failed to install go module %s@%s: %w", s.modulePath(), version, err)
		}
	}

	// Always write mcp.toml so config changes are picked up even when
	// the module version is already cached.
	if err := s.writeMCPConfig(store, segs); err != nil {
		return nil, fmt.Errorf("writing mcp config: %w", err)
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

func (s *GoSource) resolveConcreteVersion(ctx context.Context) (string, error) {
	mod := s.modulePath()
	ver := s.versionSuffix()

	// go list -m resolves any ref (latest, branch name, tag, pseudo-version)
	// to a concrete version string. This works for module-root packages.
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-f", "{{.Version}}", mod+"@"+ver)
	cmd.Env = append(cmd.Environ(), "GOWORK=off")
	out, err := cmd.Output()
	if err == nil {
		version := strings.TrimSpace(string(out))
		if version != "" {
			return version, nil
		}
	}

	// Fallback: the package path may be a subpath within a module (e.g.
	// golang.org/x/tools/cmd/stringer where the module is golang.org/x/tools).
	// In that case go list -m fails. Use the user-provided version directly;
	// go install will resolve it.
	return ver, nil
}

func (s *GoSource) getStoreSegments(resolvedVersion string) []string {
	modParts := strings.Split(s.modulePath(), "/")

	segs := make([]string, 0, 2+len(modParts))
	segs = append(segs, "go")
	segs = append(segs, modParts...)
	segs = append(segs, resolvedVersion)

	return segs
}

// modulePath returns the Go module path without the @version suffix.
func (s *GoSource) modulePath() string {
	if idx := strings.LastIndex(s.Package, "@"); idx > 0 {
		return s.Package[:idx]
	}
	return s.Package
}

// versionSuffix returns the version part after @ in the package string,
// or "latest" if no version is specified.
func (s *GoSource) versionSuffix() string {
	if idx := strings.LastIndex(s.Package, "@"); idx > 0 {
		return s.Package[idx+1:]
	}
	return "latest"
}

func (s *GoSource) install(ctx context.Context, dest string, version string) error {
	pkg := fmt.Sprintf("%s@%s", s.modulePath(), version)

	cmd := exec.CommandContext(ctx, "go", "install", pkg)
	cmd.Env = append(cmd.Environ(), "GOBIN="+dest+"/bin", "GOWORK=off")
	if _, err := cmd.Output(); err != nil {
		return execError(err)
	}

	return nil
}

func (s *GoSource) writeMCPConfig(store store.Store, segs []string) error {
	data, err := toml.Marshal(s.MCPConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal mcp config: %w", err)
	}

	mcpSegs := make([]string, len(segs)+1)
	copy(mcpSegs, segs)
	mcpSegs[len(mcpSegs)-1] = mcpFileName

	return store.WriteFile(data, mcpFilePerms, mcpSegs...)
}
