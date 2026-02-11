package projector

import (
	"github.com/agentpkg/agentpkg/pkg/pkg/skill"
)

type Projector interface {
	// SupportsSkills returns whether or not the given agent supports skills
	SupportsSkills() bool
	// Project projects the packages to the appropriate handler by type
	ProjectSkills(projectDir string, packages []skill.Skill) error
}
