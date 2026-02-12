package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentpkg/agentpkg/pkg/config"
	"github.com/pelletier/go-toml/v2"
)

const (
	mcpConfigFile  = "mcp.toml"
	transportStdio = "stdio"
	cmdNpx         = "npx"
)

type MCP interface {
	Name() string
	Validate() error

	// used by projectors to write agent-specific config
	// do not necessarily return all the underlying apkg config
	Transport() string
	Command() string
	Args() []string
	URL() string
	Headers() map[string]string
	Env() map[string]string
}

func Load(dir string) (MCP, error) {
	configFile := filepath.Join(dir, mcpConfigFile)

	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read %q: %w", configFile, err)
	}

	config := &config.MCPSource{}
	if err := toml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %q: %w", configFile, err)
	}

	if config.ManagedStdioMCPConfig != nil {
		return &managedLocalMCPServer{
			name: config.Name,
			dir:  dir,
			args: config.Args,
			env:  config.Env,
		}, nil
	}

	return nil, fmt.Errorf("unsupported MCP server configuration")
}

type managedLocalMCPServer struct {
	name string
	dir  string
	args []string
	env  map[string]string
}

func (s *managedLocalMCPServer) Name() string {
	return s.name
}

func (s *managedLocalMCPServer) Validate() error {
	if s.Command() == "" {
		return fmt.Errorf("unable to create command to run managed mcp server")
	}

	return nil
}

func (s *managedLocalMCPServer) Transport() string {
	return transportStdio
}

func (s *managedLocalMCPServer) Command() string {
	if strings.Contains(s.dir, "npm") {
		return cmdNpx
	}

	return ""
}

func (s *managedLocalMCPServer) Args() []string {
	args := make([]string, 0, len(s.args)+1)

	args = append(args, s.dir)
	args = append(args, s.args...)

	return args
}

func (s *managedLocalMCPServer) URL() string {
	return ""
}

func (s *managedLocalMCPServer) Headers() map[string]string {
	return nil
}

func (s *managedLocalMCPServer) Env() map[string]string {
	return s.env
}
