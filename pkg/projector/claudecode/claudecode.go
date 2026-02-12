package claudecode

import (
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

func (c *claudeCodeProjector) ProjectSkills(projectDir string, packages []skill.Skill) error {
	return c.sp.ProjectSkills(projectDir, packages)
}
