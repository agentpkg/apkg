package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/agentpkg/agentpkg/pkg/config"
	"github.com/agentpkg/agentpkg/pkg/store"
	"github.com/pelletier/go-toml/v2"
)

// StaticSource is a source for MCP servers that don't require installation.
// It writes the MCP config (mcp.toml) to the store and returns the directory.
// Used for unmanaged stdio servers (pre-installed commands) and external HTTP servers.
type StaticSource struct {
	Name      string
	MCPConfig config.MCPSource
}

var _ Source = &StaticSource{}

func (s *StaticSource) Fetch(ctx context.Context, store store.Store) (*ResolvedSource, error) {
	data, err := toml.Marshal(s.MCPConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal mcp config: %w", err)
	}

	segs := s.storeSegments(data)

	cached, err := store.Exists(segs...)
	if err != nil {
		return nil, fmt.Errorf("failed to check cached static source: %w", err)
	}

	if !cached {
		store.EnsureDir(segs...)

		mcpSegs := append(segs, mcpFileName)
		if err := store.WriteFile(data, mcpFilePerms, mcpSegs...); err != nil {
			store.Remove(segs...)
			return nil, fmt.Errorf("failed to write mcp config: %w", err)
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

// storeSegments returns the store path segments for this static source.
// The path is content-addressable: static/<name>/<sha256-of-config>, so
// different configs with the same name don't collide, while identical
// configs are correctly deduped.
func (s *StaticSource) storeSegments(marshaledConfig []byte) []string {
	h := sha256.Sum256(marshaledConfig)
	return []string{"static", s.Name, hex.EncodeToString(h[:])}
}
