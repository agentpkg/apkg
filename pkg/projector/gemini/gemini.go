package gemini

import (
	"github.com/agentpkg/agentpkg/pkg/pkg/skill"
	"github.com/agentpkg/agentpkg/pkg/projector"
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

func (g *geminiProjector) ProjectSkills(projectDir string, packages []skill.Skill) error {
	return g.sp.ProjectSkills(projectDir, packages)
}
