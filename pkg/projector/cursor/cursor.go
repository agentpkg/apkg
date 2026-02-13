package cursor

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentpkg/agentpkg/pkg/mcp"
	"github.com/agentpkg/agentpkg/pkg/projector"
	"github.com/agentpkg/agentpkg/pkg/skill"
)

func init() {
	projector.RegisterProjector("cursor", &cursorProjector{
		sp: projector.SkillProjector{AgentDir: ".cursor"},
	})
}

type cursorProjector struct {
	sp projector.SkillProjector
}

var _ projector.Projector = &cursorProjector{}

func (c *cursorProjector) GitignoreEntries() []string {
	return []string{".cursor/"}
}

func (c *cursorProjector) SupportsSkills() bool {
	return true
}

func (c *cursorProjector) ProjectSkills(opts projector.ProjectionOpts, packages []skill.Skill) error {
	return c.sp.ProjectSkills(opts, packages)
}

func (c *cursorProjector) UnprojectSkills(opts projector.ProjectionOpts, names []string) error {
	return c.sp.UnprojectSkills(opts, names)
}

func (c *cursorProjector) SupportsMCPServers() bool {
	return true
}

func (c *cursorProjector) ProjectMCPServers(opts projector.ProjectionOpts, servers []mcp.MCPServer) error {
	var configPath string
	if opts.Scope == projector.ScopeGlobal {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		configPath = filepath.Join(homeDir, ".cursor", "mcp.json")
	} else {
		configPath = filepath.Join(opts.ProjectDir, ".cursor", "mcp.json")
	}

	config, err := projector.ReadJsonConfig(configPath)
	if err != nil {
		return err
	}

	for _, server := range servers {
		serverConfig := projector.BuildMCPServerJsonConfig(server)
		mcpServers := projector.GetOrCreateMap(config, "mcpServers")
		mcpServers[server.Name()] = serverConfig
	}

	return projector.WriteJsonConfig(configPath, config)
}

func (c *cursorProjector) UnprojectMCPServers(opts projector.ProjectionOpts, names []string) error {
	var configPath string
	if opts.Scope == projector.ScopeGlobal {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		configPath = filepath.Join(homeDir, ".cursor", "mcp.json")
	} else {
		configPath = filepath.Join(opts.ProjectDir, ".cursor", "mcp.json")
	}

	config, err := projector.ReadJsonConfig(configPath)
	if err != nil {
		return err
	}

	if mcpServers, ok := config["mcpServers"].(map[string]any); ok {
		for _, name := range names {
			delete(mcpServers, name)
		}
	}

	return projector.WriteJsonConfig(configPath, config)
}
