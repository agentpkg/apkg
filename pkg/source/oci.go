package source

import (
	"context"
	"fmt"

	"github.com/agentpkg/agentpkg/pkg/config"
	"github.com/agentpkg/agentpkg/pkg/container"
	"github.com/agentpkg/agentpkg/pkg/store"
	"github.com/pelletier/go-toml/v2"
)

// OCISource handles container-image-based MCP servers. It pulls the image
// using the detected container engine, resolves the image digest, and writes
// an mcp.toml to the store at oci/<name>/<digest>/.
//
// The serve command later discovers installed OCI servers by scanning
// the store's oci/ directory.
type OCISource struct {
	Name      string
	MCPConfig config.MCPSource
}

var _ Source = &OCISource{}

func (s *OCISource) Fetch(ctx context.Context, st store.Store) (*ResolvedSource, error) {
	engine, err := container.DetectEngine()
	if err != nil {
		return nil, fmt.Errorf("detecting container engine: %w", err)
	}

	if err := engine.Pull(ctx, s.MCPConfig.Image); err != nil {
		return nil, fmt.Errorf("pulling image: %w", err)
	}

	digest, err := engine.ImageDigest(ctx, s.MCPConfig.Image)
	if err != nil {
		return nil, fmt.Errorf("resolving image digest: %w", err)
	}

	// Stamp the resolved digest into the config so mcp.Load can read it
	// from the persisted mcp.toml and set the routing headers.
	if s.MCPConfig.ContainerMCPConfig != nil {
		s.MCPConfig.ContainerMCPConfig.Digest = digest
		if s.MCPConfig.ContainerMCPConfig.Path == "" {
			s.MCPConfig.ContainerMCPConfig.Path = "/mcp"
		}
	}

	segs := []string{"oci", s.Name, digest}

	cached, err := st.Exists(segs...)
	if err != nil {
		return nil, fmt.Errorf("checking cached OCI source: %w", err)
	}

	if !cached {
		st.EnsureDir(segs...)
	}

	// Always write mcp.toml so config changes are picked up even when
	// the image digest is already cached.
	if err := s.writeMCPConfig(st, segs); err != nil {
		return nil, fmt.Errorf("writing mcp config: %w", err)
	}

	integrity, err := st.HashDir(segs...)
	if err != nil {
		return nil, fmt.Errorf("computing integrity hash: %w", err)
	}

	return &ResolvedSource{
		Dir:       st.Path(segs...),
		Integrity: integrity,
	}, nil
}

func (s *OCISource) writeMCPConfig(st store.Store, segs []string) error {
	data, err := toml.Marshal(s.MCPConfig)
	if err != nil {
		return fmt.Errorf("marshaling mcp config: %w", err)
	}

	mcpSegs := make([]string, len(segs)+1)
	copy(mcpSegs, segs)
	mcpSegs[len(mcpSegs)-1] = mcpFileName

	return st.WriteFile(data, mcpFilePerms, mcpSegs...)
}
