package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

const (
	dirPerm      = 0o755
	hashPrefix   = "sha256:"
	DefaultRoot  = ".apkg"
)

type Store interface {
	// Path returns the absolute filesystem path for the given segments
	// joined under the store root. Does not create or verify the path.
	// Use this to get a path for external tools (e.g., git clone target).
	Path(segments ...string) string
	// Exists reports whether the path at the given segments exists.
	Exists(segments ...string) (bool, error)
	// EnsureDir creates the directory at segments (starting at store root),
	// including parents.
	EnsureDir(segments ...string)
	// Remove deletes the entire tree at segments.
	Remove(segments ...string)
	// HashDir computes a "sha256:<hex>" integrity hash over all file
	// contents in the directory at segments, walking recursively in sorted
	// order for determinism.
	HashDir(segments ...string) (string, error)
	// WriteFile writes data to the file at segments.
	// Parent directories must already exist.
	WriteFile(data []byte, perm os.FileMode, segments ...string) error
	// ReadFile reads the file at segments.
	ReadFile(segments ...string) ([]byte, error)
}

func New(root string) Store {
	return &store{root: root}
}

func Default() (Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("determining home directory: %w", err)
	}
	return &store{root: filepath.Join(home, DefaultRoot)}, nil
}

type store struct {
	root string
}

var _ Store = &store{}

func (s *store) Path(segments ...string) string {
	return filepath.Join(append([]string{s.root}, segments...)...)
}

func (s *store) Exists(segments ...string) (bool, error) {
	_, err := os.Stat(s.Path(segments...))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (s *store) EnsureDir(segments ...string) {
	os.MkdirAll(s.Path(segments...), dirPerm)
}

func (s *store) Remove(segments ...string) {
	os.RemoveAll(s.Path(segments...))
}

func (s *store) HashDir(segments ...string) (string, error) {
	dir := s.Path(segments...)
	h := sha256.New()

	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	sort.Strings(files)

	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			return "", err
		}
		h.Write([]byte(f))
		h.Write(data)
	}

	return hashPrefix + hex.EncodeToString(h.Sum(nil)), nil
}

func (s *store) WriteFile(data []byte, perm os.FileMode, segments ...string) error {
	return os.WriteFile(s.Path(segments...), data, perm)
}

func (s *store) ReadFile(segments ...string) ([]byte, error) {
	return os.ReadFile(s.Path(segments...))
}
