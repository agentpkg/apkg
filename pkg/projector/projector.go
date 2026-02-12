package projector

import (
	"github.com/agentpkg/agentpkg/pkg/mcp"
	"github.com/agentpkg/agentpkg/pkg/skill"
)

type Scope int

const (
	ScopeLocal Scope = iota
	ScopeGlobal
)

type ProjectionOpts struct {
	ProjectDir string
	Scope      Scope
}

type Projector interface {
	// SupportsSkills returns whether or not the given agent supports skills
	SupportsSkills() bool
	// Project projects the packages to the appropriate handler by type
	ProjectSkills(opts ProjectionOpts, packages []skill.Skill) error

	// SupportsMCPServers returns whether or not the given agent supports MCP servers
	SupportsMCPServers() bool
	ProjectMCPServers(opts ProjectionOpts, servers []mcp.MCPServer) error
}
