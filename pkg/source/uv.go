package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/agentpkg/agentpkg/pkg/config"
	"github.com/agentpkg/agentpkg/pkg/store"
	"github.com/pelletier/go-toml/v2"
)

type UVSource struct {
	Package   string
	MCPConfig config.MCPSource
}

var _ Source = &UVSource{}

func (s *UVSource) Fetch(ctx context.Context, store store.Store) (*ResolvedSource, error) {
	version, err := s.resolveConcreteVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get concrete version for uv package: %w", err)
	}

	segs := s.getStoreSegments(version)

	cached, err := store.Exists(segs...)
	if err != nil {
		return nil, fmt.Errorf("failed to check cached uv package: %w", err)
	}

	if !cached {
		store.EnsureDir(segs...)
		path := store.Path(segs...)

		if err := s.install(ctx, path, version); err != nil {
			store.Remove(segs...)
			return nil, fmt.Errorf("failed to install uv package %s==%s: %w", s.packageName(), version, err)
		}
	}

	// Always write mcp.toml so config changes are picked up even when
	// the package version is already cached.
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

func (s *UVSource) resolveConcreteVersion(ctx context.Context) (string, error) {
	// if the package spec contains ==, extract the pinned version directly
	if idx := strings.Index(s.Package, "=="); idx >= 0 {
		return s.Package[idx+2:], nil
	}

	// otherwise query PyPI JSON API for the latest version
	url := fmt.Sprintf("https://pypi.org/pypi/%s/json", s.packageName())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating pypi request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("querying pypi for %s: %w", s.packageName(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("pypi returned status %d for %s", resp.StatusCode, s.packageName())
	}

	var result struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding pypi response for %s: %w", s.packageName(), err)
	}

	if result.Info.Version == "" {
		return "", fmt.Errorf("no version found for %s on pypi", s.packageName())
	}

	return result.Info.Version, nil
}

func (s *UVSource) getStoreSegments(resolvedVersion string) []string {
	return []string{"uv", s.packageName(), resolvedVersion}
}

func (s *UVSource) packageName() string {
	if idx := strings.Index(s.Package, "=="); idx >= 0 {
		return s.Package[:idx]
	}
	return s.Package
}

func (s *UVSource) install(ctx context.Context, dest string, version string) error {
	venvPath := dest + "/.venv"

	cmd := exec.CommandContext(ctx, "uv", "venv", venvPath)
	if _, err := cmd.Output(); err != nil {
		return fmt.Errorf("creating venv: %w", execError(err))
	}

	pkg := fmt.Sprintf("%s==%s", s.packageName(), version)
	cmd = exec.CommandContext(ctx, "uv", "pip", "install", "--python", venvPath+"/bin/python", pkg)
	if _, err := cmd.Output(); err != nil {
		return fmt.Errorf("installing package: %w", execError(err))
	}

	return nil
}

func (s *UVSource) writeMCPConfig(store store.Store, segs []string) error {
	data, err := toml.Marshal(s.MCPConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal mcp config: %w", err)
	}

	mcpSegs := make([]string, len(segs)+1)
	copy(mcpSegs, segs)
	mcpSegs[len(mcpSegs)-1] = mcpFileName

	return store.WriteFile(data, mcpFilePerms, mcpSegs...)
}
