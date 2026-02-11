package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// ManifestFileName is the manifest filename used for both project-local
// and global configurations.
const ManifestFileName = "apkg.toml"

type Config struct {
	Project ProjectConfig          `toml:"project"`
	Skills  map[string]SkillSource `toml:"skills,omitempty,inline"`
}

type ProjectConfig struct {
	Name string `toml:"name"`
}

type SkillSource struct {
	// Short form: "owner/repo/path@ref"
	Short string `toml:"-"`

	Git  string `toml:"git,omitempty"`
	Path string `toml:"path,omitempty"`
	Ref  string `toml:"ref,omitempty"`
}

func UnmarshalConfig(data []byte) (*Config, error) {
	cfg := &Config{}
	err := toml.Unmarshal(data, cfg)

	return cfg, err
}

func (c *Config) Marshal() ([]byte, error) {
	return toml.Marshal(c)
}

func LoadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return UnmarshalConfig(data)
}

func SaveFile(path string, cfg *Config) error {
	data, err := cfg.Marshal()
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// GlobalManifestPath returns the path to the global manifest (~/.apkg/apkg.toml),
// ensuring the directory exists.
func GlobalManifestPath() (string, error) {
	dir, err := GlobalConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ManifestFileName), nil
}
