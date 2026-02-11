package store

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPath(t *testing.T) {
	root := "/tmp/store-root"

	tests := map[string]struct {
		segments []string
		want     string
	}{
		"no segments": {
			segments: nil,
			want:     root,
		},
		"single segment": {
			segments: []string{"foo"},
			want:     filepath.Join(root, "foo"),
		},
		"multiple segments": {
			segments: []string{"foo", "bar", "baz"},
			want:     filepath.Join(root, "foo", "bar", "baz"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := New(root)
			got := s.Path(tc.segments...)
			if got != tc.want {
				t.Errorf("Path(%v) = %q, want %q", tc.segments, got, tc.want)
			}
		})
	}
}

func TestExists(t *testing.T) {
	root := t.TempDir()
	s := New(root)

	existingDir := "existing-dir"
	os.MkdirAll(filepath.Join(root, existingDir), 0o755)

	existingFile := "existing-file.txt"
	os.WriteFile(filepath.Join(root, existingFile), []byte("hello"), 0o644)

	tests := map[string]struct {
		segments []string
		want     bool
	}{
		"existing directory": {
			segments: []string{existingDir},
			want:     true,
		},
		"existing file": {
			segments: []string{existingFile},
			want:     true,
		},
		"non-existent path": {
			segments: []string{"does-not-exist"},
			want:     false,
		},
		"nested non-existent path": {
			segments: []string{"a", "b", "c"},
			want:     false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := s.Exists(tc.segments...)
			if err != nil {
				t.Fatalf("Exists(%v) returned unexpected error: %v", tc.segments, err)
			}
			if got != tc.want {
				t.Errorf("Exists(%v) = %v, want %v", tc.segments, got, tc.want)
			}
		})
	}
}

func TestEnsureDir(t *testing.T) {
	tests := map[string]struct {
		segments []string
	}{
		"single level": {
			segments: []string{"alpha"},
		},
		"nested levels": {
			segments: []string{"alpha", "beta", "gamma"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			s := New(root)

			s.EnsureDir(tc.segments...)

			dir := filepath.Join(append([]string{root}, tc.segments...)...)
			info, err := os.Stat(dir)
			if err != nil {
				t.Fatalf("directory was not created: %v", err)
			}
			if !info.IsDir() {
				t.Error("path exists but is not a directory")
			}
		})
	}
}

func TestRemove(t *testing.T) {
	tests := map[string]struct {
		setup func(root string)
		// segments to remove
		segments []string
	}{
		"remove file": {
			setup: func(root string) {
				os.WriteFile(filepath.Join(root, "file.txt"), []byte("data"), 0o644)
			},
			segments: []string{"file.txt"},
		},
		"remove directory tree": {
			setup: func(root string) {
				dir := filepath.Join(root, "a", "b")
				os.MkdirAll(dir, 0o755)
				os.WriteFile(filepath.Join(dir, "c.txt"), []byte("nested"), 0o644)
			},
			segments: []string{"a"},
		},
		"remove non-existent path": {
			setup:    func(root string) {},
			segments: []string{"ghost"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			s := New(root)
			tc.setup(root)

			s.Remove(tc.segments...)

			target := filepath.Join(append([]string{root}, tc.segments...)...)
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Errorf("expected path %q to be removed", target)
			}
		})
	}
}

func TestWriteFileReadFile(t *testing.T) {
	tests := map[string]struct {
		segments []string
		data     []byte
		perm     os.FileMode
	}{
		"simple file at root": {
			segments: []string{"hello.txt"},
			data:     []byte("hello world"),
			perm:     0o644,
		},
		"nested file": {
			segments: []string{"sub", "dir", "data.bin"},
			data:     []byte{0x00, 0xFF, 0xAB},
			perm:     0o600,
		},
		"empty file": {
			segments: []string{"empty"},
			data:     []byte{},
			perm:     0o644,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			s := New(root)

			// ensure parent directories exist
			if len(tc.segments) > 1 {
				s.EnsureDir(tc.segments[:len(tc.segments)-1]...)
			}

			if err := s.WriteFile(tc.data, tc.perm, tc.segments...); err != nil {
				t.Fatalf("WriteFile() error: %v", err)
			}

			got, err := s.ReadFile(tc.segments...)
			if err != nil {
				t.Fatalf("ReadFile() error: %v", err)
			}

			if string(got) != string(tc.data) {
				t.Errorf("ReadFile() = %q, want %q", got, tc.data)
			}
		})
	}
}

func TestReadFileNotFound(t *testing.T) {
	root := t.TempDir()
	s := New(root)

	_, err := s.ReadFile("nonexistent.txt")
	if err == nil {
		t.Fatal("expected error reading nonexistent file, got nil")
	}
}

func TestHashDir(t *testing.T) {
	// helper to compute the expected hash given a sorted list of (relativePath, content) pairs
	computeExpected := func(pairs [][2]string) string {
		h := sha256.New()
		for _, p := range pairs {
			h.Write([]byte(p[0]))
			h.Write([]byte(p[1]))
		}
		return hashPrefix + hex.EncodeToString(h.Sum(nil))
	}

	tests := map[string]struct {
		// files maps relative path (forward-slash separated) to content
		files map[string]string
		// pairs is the sorted list used to compute the expected hash
		pairs [][2]string
	}{
		"single file": {
			files: map[string]string{
				"a.txt": "alpha",
			},
			pairs: [][2]string{
				{"a.txt", "alpha"},
			},
		},
		"multiple files sorted order": {
			files: map[string]string{
				"b.txt": "bravo",
				"a.txt": "alpha",
				"c.txt": "charlie",
			},
			pairs: [][2]string{
				{"a.txt", "alpha"},
				{"b.txt", "bravo"},
				{"c.txt", "charlie"},
			},
		},
		"nested files": {
			files: map[string]string{
				filepath.Join("sub", "z.txt"): "zulu",
				"a.txt":                       "alpha",
			},
			pairs: [][2]string{
				{"a.txt", "alpha"},
				{filepath.Join("sub", "z.txt"), "zulu"},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			s := New(root)

			dir := "hashme"
			base := filepath.Join(root, dir)

			for relPath, content := range tc.files {
				full := filepath.Join(base, relPath)
				os.MkdirAll(filepath.Dir(full), 0o755)
				os.WriteFile(full, []byte(content), 0o644)
			}

			got, err := s.HashDir(dir)
			if err != nil {
				t.Fatalf("HashDir() error: %v", err)
			}

			want := computeExpected(tc.pairs)
			if got != want {
				t.Errorf("HashDir() = %q, want %q", got, want)
			}

			if !strings.HasPrefix(got, hashPrefix) {
				t.Errorf("HashDir() result missing %q prefix", hashPrefix)
			}
		})
	}
}

func TestHashDirDeterminism(t *testing.T) {
	root := t.TempDir()
	s := New(root)

	dir := "det"
	base := filepath.Join(root, dir)
	os.MkdirAll(base, 0o755)
	os.WriteFile(filepath.Join(base, "x.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(base, "y.txt"), []byte("y"), 0o644)

	hash1, err := s.HashDir(dir)
	if err != nil {
		t.Fatalf("first HashDir() error: %v", err)
	}

	hash2, err := s.HashDir(dir)
	if err != nil {
		t.Fatalf("second HashDir() error: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("HashDir() not deterministic: %q != %q", hash1, hash2)
	}
}

func TestHashDirNonExistent(t *testing.T) {
	root := t.TempDir()
	s := New(root)

	_, err := s.HashDir("no-such-dir")
	if err == nil {
		t.Fatal("expected error hashing nonexistent directory, got nil")
	}
}
