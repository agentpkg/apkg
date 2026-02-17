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
	Project    ProjectConfig          `toml:"project"`
	Skills     map[string]SkillSource `toml:"skills,omitempty"`
	MCPServers map[string]MCPSource   `toml:"mcpServers,omitempty"`
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

type MCPSource struct {
	// Transport is required: "stdio" or "http"
	Transport string `toml:"transport"`

	// Name of the server, overrides the key in the table of mcp servers
	Name string `toml:"name,omitempty"`

	// container config
	*ContainerMCPConfig `toml:",omitempty"`
	// external http server config
	*ExternalHttpMCPConfig `toml:",omitempty"`
	// managed stdio config
	*ManagedStdioMCPConfig `toml:",omitempty"` // TODO: implement support
	// unmanaged stdio config
	*UnmanagedStdioMCPConfig `toml:",omitempty"`

	// http transport config
	*HttpMCPConfig `toml:",omitempty"`

	// common config for any locally-run mcp server (stdio or container)
	*LocalMCPConfig `toml:",omitempty"`
}

type ContainerMCPConfig struct {
	Image   string   `toml:"image,omitempty"`
	Port    *int     `toml:"port,omitempty"`    // port within container image to map to
	Path    string   `toml:"path,omitempty"`    // URL path on the container (default "mcp")
	Digest  string   `toml:"digest,omitempty"`  // resolved image digest, populated at install time
	Volumes []string `toml:"volumes,omitempty"` // bind mounts (host:container[:ro])
	Network string   `toml:"network,omitempty"` // container network (e.g. "host", "kind")
}

// config for any http transport mcp server
type HttpMCPConfig struct {
	Headers map[string]string `toml:"headers,omitempty"`
}

// config for external http server
type ExternalHttpMCPConfig struct {
	URL string `toml:"url,omitempty"`
}

// config for managed stdio mcp server
type ManagedStdioMCPConfig struct {
	// managed package - apkg installs + pins locally
	// Format: "npm:<package>[@version]", "uv:<package>[==version]", or "go:<module>[@version]"
	Package string `toml:"package,omitempty"`

	// Runtime is the resolved absolute path to the interpreter needed to
	// run the package (e.g. /usr/local/bin/node for npm packages). It is
	// populated at install time so that agents which do not source the
	// shell environment (e.g. Cursor) can locate the runtime.
	Runtime string `toml:"runtime,omitempty"`
}

// config for unmanaged stdio mcp server
type UnmanagedStdioMCPConfig struct {
	Command string `toml:"command,omitempty"`
}

type LocalMCPConfig struct {
	Env  map[string]string `toml:"env,omitempty"`
	Args []string          `toml:"args,omitempty"`
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
