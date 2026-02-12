package source

import (
	"context"

	"github.com/agentpkg/agentpkg/pkg/store"
)

type Source interface {
	// Fetch retrieves the source content into the store (or validates local path).
	// Returns the resolved source with path, commit, and integrity for the lockfile.
	Fetch(ctx context.Context, store store.Store) (*ResolvedSource, error)
}

type ResolvedSource struct {
	Dir       string // Path to package content on disk
	Commit    string // Resolved commit hash (git only)
	Ref       string // Original ref (git only)
	Integrity string // SHA256 of directory contents (empty for local)
}
