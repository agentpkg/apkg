package projector

import (
	"testing"

	"github.com/agentpkg/agentpkg/pkg/mcp"
	"github.com/agentpkg/agentpkg/pkg/skill"
)

type stubProjector struct{}

func (s *stubProjector) SupportsSkills() bool                                          { return true }
func (s *stubProjector) ProjectSkills(_ ProjectionOpts, _ []skill.Skill) error         { return nil }
func (s *stubProjector) UnprojectSkills(_ ProjectionOpts, _ []string) error            { return nil }
func (s *stubProjector) SupportsMCPServers() bool                                      { return true }
func (s *stubProjector) ProjectMCPServers(_ ProjectionOpts, _ []mcp.MCPServer) error   { return nil }
func (s *stubProjector) UnprojectMCPServers(_ ProjectionOpts, _ []string) error        { return nil }

func TestRegisteredAgents(t *testing.T) {
	tests := map[string]struct {
		setup func()
		want  []string
	}{
		"empty registry": {
			setup: func() {
				defaultRegistry = make(registry)
			},
			want: nil,
		},
		"single agent": {
			setup: func() {
				defaultRegistry = registry{
					"claude-code": &stubProjector{},
				}
			},
			want: []string{"claude-code"},
		},
		"multiple agents sorted": {
			setup: func() {
				defaultRegistry = registry{
					"cursor":      &stubProjector{},
					"claude-code": &stubProjector{},
					"aider":       &stubProjector{},
				}
			},
			want: []string{"aider", "claude-code", "cursor"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.setup()
			got := RegisteredAgents()

			if len(got) != len(tc.want) {
				t.Fatalf("RegisteredAgents() returned %d agents, want %d", len(got), len(tc.want))
			}
			for i, agent := range got {
				if agent != tc.want[i] {
					t.Errorf("RegisteredAgents()[%d] = %q, want %q", i, agent, tc.want[i])
				}
			}
		})
	}
}
