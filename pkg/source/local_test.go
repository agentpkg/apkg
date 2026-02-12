package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentpkg/agentpkg/pkg/store"
)

func TestLocalSourceFetch(t *testing.T) {
	existingDir := t.TempDir()

	tests := map[string]struct {
		path    string
		wantErr bool
	}{
		"valid directory": {
			path:    existingDir,
			wantErr: false,
		},
		"nonexistent path": {
			path:    filepath.Join(t.TempDir(), "does-not-exist"),
			wantErr: true,
		},
		"file not directory": {
			path: func() string {
				f := filepath.Join(t.TempDir(), "afile")
				os.WriteFile(f, []byte("hi"), 0o644)
				return f
			}(),
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := store.New(t.TempDir())
			src := &LocalSource{Path: tc.path}

			result, err := src.Fetch(context.Background(), s)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Fetch() error = %v, wantErr = %v", err, tc.wantErr)
			}

			if err == nil {
				if !filepath.IsAbs(result.Dir) {
					t.Errorf("Dir = %q, want absolute path", result.Dir)
				}
				if result.Commit != "" {
					t.Errorf("Commit = %q, want empty for local source", result.Commit)
				}
				if result.Integrity != "" {
					t.Errorf("Integrity = %q, want empty for local source", result.Integrity)
				}
			}
		})
	}
}

func TestLocalSourceFetchRelativePath(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")
	os.MkdirAll(subDir, 0o755)

	// Change to dir so that "./sub" resolves correctly.
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(origWd) })

	s := store.New(t.TempDir())
	src := &LocalSource{Path: "./sub"}

	result, err := src.Fetch(context.Background(), s)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
	if !filepath.IsAbs(result.Dir) {
		t.Errorf("Dir = %q, want absolute path", result.Dir)
	}
}
