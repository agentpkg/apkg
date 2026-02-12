package gemini

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentpkg/agentpkg/pkg/mcp"
	"github.com/agentpkg/agentpkg/pkg/projector"
	"github.com/agentpkg/agentpkg/pkg/skill"
)

func init() {
	projector.RegisterProjector("gemini", &geminiProjector{
		sp: projector.SkillProjector{AgentDir: ".gemini"},
	})
}

type geminiProjector struct {
	sp projector.SkillProjector
}

var _ projector.Projector = &geminiProjector{}

func (g *geminiProjector) SupportsSkills() bool {
	return true
}

func (g *geminiProjector) ProjectSkills(opts projector.ProjectionOpts, packages []skill.Skill) error {
	return g.sp.ProjectSkills(opts, packages)
}

func (g *geminiProjector) SupportsMCPServers() bool {
	return true
}

func (g *geminiProjector) ProjectMCPServers(opts projector.ProjectionOpts, servers []mcp.MCPServer) error {
	var configPath string
	if opts.Scope == projector.ScopeGlobal {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}

		configPath = filepath.Join(homeDir, ".gemini", "settings.json")
	} else {
		configPath = filepath.Join(opts.ProjectDir, ".gemini", "settings.json")
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
