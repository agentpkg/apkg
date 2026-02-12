package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentpkg/agentpkg/pkg/store"
)

type LocalSource struct {
	Path string
}

var _ Source = &LocalSource{}

func (l *LocalSource) Fetch(ctx context.Context, s store.Store) (*ResolvedSource, error) {
	absPath, err := filepath.Abs(l.Path)
	if err != nil {
		return nil, fmt.Errorf("resolving absolute path for %q: %w", l.Path, err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("local source path does not exist: %s", absPath)
		}
		return nil, fmt.Errorf("checking local source path %s: %w", absPath, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("local source path is not a directory: %s", absPath)
	}

	return &ResolvedSource{
		Dir: absPath,
	}, nil
}

