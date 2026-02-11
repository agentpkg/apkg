package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/viper"
)

// LocalConfigFile is the project-local developer config filename.
const LocalConfigFile = "apkg.local.toml"

// DevConfig holds developer-specific configuration that is NOT committed
// to version control. It is resolved with Viper precedence:
// CLI flags > apkg.local.toml (project-local) > ~/.apkg/config.toml (global).
type DevConfig struct {
	Agents []string `toml:"agents" mapstructure:"agents"`
}

// LoadDevConfig resolves developer configuration using Viper's merge semantics.
// When global is true, only the global config (~/.apkg/config.toml) is loaded,
// skipping the project-local apkg.local.toml. This ensures that global installs
// use global agent preferences rather than project-scoped ones.
// flagAgents, if non-empty, takes highest precedence (set via --agents flag).
func LoadDevConfig(flagAgents []string, global bool) (*DevConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("determining home directory: %w", err)
	}
	globalPath := filepath.Join(home, ".apkg", "config.toml")
	return loadDevConfig(flagAgents, global, globalPath, LocalConfigFile)
}

// loadDevConfig is the internal implementation that accepts explicit paths,
// making it testable without touching the real home directory.
func loadDevConfig(flagAgents []string, global bool, globalPath, localPath string) (*DevConfig, error) {
	v := viper.New()
	v.SetConfigType("toml")

	// Lowest priority: global config
	v.SetConfigFile(globalPath)
	// Read global config; ignore if missing.
	_ = v.ReadInConfig()

	// Higher priority: project-local config (skipped for global scope)
	if !global {
		if _, err := os.Stat(localPath); err == nil {
			v.SetConfigFile(localPath)
			if err := v.MergeInConfig(); err != nil {
				return nil, fmt.Errorf("reading %s: %w", localPath, err)
			}
		}
	}

	// Highest priority: CLI flags
	if len(flagAgents) > 0 {
		v.Set("agents", flagAgents)
	}

	cfg := &DevConfig{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling dev config: %w", err)
	}

	return cfg, nil
}

// GlobalConfigDir returns the path to ~/.apkg, creating it if necessary.
func GlobalConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determining home directory: %w", err)
	}
	dir := filepath.Join(home, ".apkg")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating %s: %w", dir, err)
	}
	return dir, nil
}

// WriteLocalDevConfig persists developer config to apkg.local.toml in the
// given project directory.
func WriteLocalDevConfig(projectDir string, cfg *DevConfig) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling dev config: %w", err)
	}

	path := filepath.Join(projectDir, LocalConfigFile)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	return nil
}

// WriteGlobalDevConfig persists developer config to ~/.apkg/config.toml.
func WriteGlobalDevConfig(cfg *DevConfig) error {
	dir, err := GlobalConfigDir()
	if err != nil {
		return err
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling dev config: %w", err)
	}

	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	return nil
}
