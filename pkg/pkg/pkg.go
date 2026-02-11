package pkg

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentpkg/agentpkg/pkg/pkg/skill"
)

type Package interface {
	// Name returns the name of the package
	Name() string
	// Type returns the type of the package (e.g. "skill", or "mcp" in the future)
	Type() string
	// Dir returns where the package contents lives on disk
	Dir() string
	// Validate makes sure package contents are okay
	Validate() error
}

func Load(dir string) (Package, error) {
	if fileExists(filepath.Join(dir, "SKILL.md")) {
		return skill.Load(dir)
	}

	return nil, fmt.Errorf("no recognized package format in %q", dir)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}

	return false
}
