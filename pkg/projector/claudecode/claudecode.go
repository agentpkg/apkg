package claudecode

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentpkg/agentpkg/pkg/mcp"
	"github.com/agentpkg/agentpkg/pkg/projector"
	"github.com/agentpkg/agentpkg/pkg/skill"
)

func init() {
	projector.RegisterProjector("claude-code", &claudeCodeProjector{
		sp: projector.SkillProjector{AgentDir: ".claude"},
	})
}

type claudeCodeProjector struct {
	sp projector.SkillProjector
}

var _ projector.Projector = &claudeCodeProjector{}

func (c *claudeCodeProjector) SupportsSkills() bool {
	return true
}

func (c *claudeCodeProjector) ProjectSkills(opts projector.ProjectionOpts, packages []skill.Skill) error {
	return c.sp.ProjectSkills(opts, packages)
}

func (c *claudeCodeProjector) UnprojectSkills(opts projector.ProjectionOpts, names []string) error {
	return c.sp.UnprojectSkills(opts, names)
}

func (c *claudeCodeProjector) SupportsMCPServers() bool {
	return true
}

func (c *claudeCodeProjector) ProjectMCPServers(opts projector.ProjectionOpts, servers []mcp.MCPServer) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	claudeConfigPath := filepath.Join(homeDir, ".claude.json")

	config, err := projector.ReadJsonConfig(claudeConfigPath)
	if err != nil {
		return err
	}

	projectDir, err := filepath.Abs(opts.ProjectDir)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path for project dir %q: %w", opts.ProjectDir, err)
	}

	for _, server := range servers {
		serverConfig := projector.BuildMCPServerJsonConfig(server)

		if opts.Scope == projector.ScopeGlobal {
			mcpServers := projector.GetOrCreateMap(config, "mcpServers")
			mcpServers[server.Name()] = serverConfig
		} else {
			projects := projector.GetOrCreateMap(config, "projects")
			project := projector.GetOrCreateMap(projects, projectDir)
			mcpServers := projector.GetOrCreateMap(project, "mcpServers")
			mcpServers[server.Name()] = serverConfig
		}
	}

	return projector.WriteJsonConfig(claudeConfigPath, config)
}

func (c *claudeCodeProjector) UnprojectMCPServers(opts projector.ProjectionOpts, names []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	claudeConfigPath := filepath.Join(homeDir, ".claude.json")

	config, err := projector.ReadJsonConfig(claudeConfigPath)
	if err != nil {
		return err
	}

	projectDir, err := filepath.Abs(opts.ProjectDir)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path for project dir %q: %w", opts.ProjectDir, err)
	}

	for _, name := range names {
		if opts.Scope == projector.ScopeGlobal {
			if mcpServers, ok := config["mcpServers"].(map[string]any); ok {
				delete(mcpServers, name)
			}
		} else {
			if projects, ok := config["projects"].(map[string]any); ok {
				if project, ok := projects[projectDir].(map[string]any); ok {
					if mcpServers, ok := project["mcpServers"].(map[string]any); ok {
						delete(mcpServers, name)
					}
				}
			}
		}
	}

	return projector.WriteJsonConfig(claudeConfigPath, config)
}
