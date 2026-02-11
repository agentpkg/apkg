package projector

import (
	"fmt"
	"sort"
)

// registry tracks Projector instances for different agents
type registry map[string]Projector

var (
	defaultRegistry = make(registry)
)

// RegisteredAgents returns a sorted list of all registered agent names.
func RegisteredAgents() []string {
	agents := make([]string, 0, len(defaultRegistry))
	for name := range defaultRegistry {
		agents = append(agents, name)
	}
	sort.Strings(agents)
	return agents
}

func GetProjector(agent string) (Projector, bool) {
	proj, ok := defaultRegistry[agent]
	return proj, ok
}

// RegisterProjector registers a projector for a given agent
// Note: this is NOT thread safe, and should only be called in init()
func RegisterProjector(agent string, proj Projector) error {
	if _, ok := defaultRegistry[agent]; ok {
		return fmt.Errorf("failed to registery projector for agent %q: other projector already registered", agent)
	}

	defaultRegistry[agent] = proj

	return nil
}
