package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentpkg/agentpkg/pkg/config"
)

const ManifestFile = config.ManifestFileName

// AgentDirs are common coding-agent directories that should typically be
// gitignored.
var AgentDirs = []string{
	".claude/",
	".cursor/",
	".gemini/",
	".agents/",
	".codex/",
}

// InferName derives a project name from the given directory path.
func InferName(dir string) string {
	return filepath.Base(dir)
}

// Init creates an apkg.toml manifest in dir with the given project name.
// Returns an error if the manifest already exists.
func Init(dir, name string) error {
	path := filepath.Join(dir, ManifestFile)

	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", ManifestFile)
	}

	cfg := &config.Config{
		Project: config.ProjectConfig{Name: name},
		Skills:  map[string]config.SkillSource{},
	}

	data, err := cfg.Marshal()
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	return nil
}

// InitGlobal creates the global manifest at ~/.apkg/apkg.toml if it does not
// already exist. It is called lazily when adding the first global skill.
func InitGlobal() error {
	manifestPath, err := config.GlobalManifestPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(manifestPath); err == nil {
		return nil // already exists
	}

	cfg := &config.Config{
		Project: config.ProjectConfig{Name: "global"},
		Skills:  map[string]config.SkillSource{},
	}

	data, err := cfg.Marshal()
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	return os.WriteFile(manifestPath, data, 0o644)
}

// EnsureGitignore ensures that each entry appears somewhere in the .gitignore
// file within dir. Only entries not already present are appended. Returns the
// list of entries that were actually added.
func EnsureGitignore(dir string, entries []string) ([]string, error) {
	path := filepath.Join(dir, ".gitignore")

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	present := make(map[string]bool)
	for _, line := range strings.Split(string(existing), "\n") {
		present[strings.TrimSpace(line)] = true
	}

	var toAdd []string
	for _, entry := range entries {
		if !present[entry] {
			toAdd = append(toAdd, entry)
		}
	}

	if len(toAdd) == 0 {
		return nil, nil
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	// Ensure we start on a new line if file doesn't end with one.
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return nil, err
		}
	}

	for _, entry := range toAdd {
		if _, err := f.WriteString(entry + "\n"); err != nil {
			return nil, err
		}
	}

	return toAdd, nil
}
