package installer

import (
	"context"
	"fmt"
	"sort"

	"github.com/agentpkg/agentpkg/pkg/config"
	"github.com/agentpkg/agentpkg/pkg/mcp"
	"github.com/agentpkg/agentpkg/pkg/projector"
	"github.com/agentpkg/agentpkg/pkg/skill"
	"github.com/agentpkg/agentpkg/pkg/source"
	"github.com/agentpkg/agentpkg/pkg/store"
)

type Installer struct {
	Store      store.Store
	ProjectDir string
	Agents     []string
	Global     bool
}

// InstallAll resolves and installs all skills from the config. It compares
// the config against the existing lockfile to avoid redundant network calls:
// if a skill's ref hasn't changed and the lockfile has a resolved commit,
// the locked commit is used directly so GitSource.Fetch only checks the
// local cache. Returns a new lockfile capturing the resolved state.
func (inst *Installer) InstallAll(ctx context.Context, cfg *config.Config, existing *config.LockFile) (*config.LockFile, error) {
	lockIndex := buildLockIndex(existing)
	lf := &config.LockFile{Version: 1}

	// Sort skill names for deterministic ordering.
	names := make([]string, 0, len(cfg.Skills))
	for name := range cfg.Skills {
		names = append(names, name)
	}
	sort.Strings(names)

	var skills []skill.Skill
	for _, name := range names {
		ss := cfg.Skills[name]
		src := source.SourceFromSkillConfig(ss)

		// If the lockfile already has a resolved commit for this skill and
		// the config ref hasn't changed, substitute the locked commit as
		// the ref. resolveRef returns full commit hashes as-is (no network
		// call), and GitSource.Fetch will find the content in the local
		// cache — making the entire fetch a local-only operation.
		key := lockKey(ss)
		if entry, ok := lockIndex[key]; ok && entry.Commit != "" && entry.Ref == ss.Ref {
			src = source.SourceFromSkillConfig(config.SkillSource{
				Git:  ss.Git,
				Path: ss.Path,
				Ref:  entry.Commit,
			})
		}

		resolved, err := src.Fetch(ctx, inst.Store)
		if err != nil {
			return nil, fmt.Errorf("fetching skill %q: %w", name, err)
		}

		s, err := skill.Load(resolved.Dir)
		if err != nil {
			return nil, fmt.Errorf("loading skill %q: %w", name, err)
		}

		if err := s.Validate(); err != nil {
			return nil, fmt.Errorf("validating skill %q: %w", name, err)
		}

		skills = append(skills, s)

		lf.Skills = append(lf.Skills, lockEntryFromResolved(ss, resolved))
	}

	if err := inst.projectSkills(skills); err != nil {
		return nil, err
	}

	// Install MCP servers.
	var servers []mcp.MCPServer
	for name, ms := range cfg.MCPServers {
		src, err := source.SourceFromMCPConfig(name, ms)
		if err != nil {
			return nil, fmt.Errorf("resolving MCP server %q: %w", name, err)
		}

		resolved, err := src.Fetch(ctx, inst.Store)
		if err != nil {
			return nil, fmt.Errorf("fetching MCP server %q: %w", name, err)
		}

		server, err := mcp.Load(resolved.Dir)
		if err != nil {
			return nil, fmt.Errorf("loading MCP server %q: %w", name, err)
		}

		if err := server.Validate(); err != nil {
			return nil, fmt.Errorf("validating MCP server %q: %w", name, err)
		}

		servers = append(servers, server)

		lf.MCPServers = append(lf.MCPServers, mcpLockEntryFromResolved(name, ms, resolved))
	}

	sort.Slice(lf.MCPServers, func(i, j int) bool {
		return lf.MCPServers[i].Name < lf.MCPServers[j].Name
	})

	if err := inst.projectMCPServers(servers); err != nil {
		return nil, err
	}

	return lf, nil
}

// InstallSkill fetches a single source, loads and validates the skill, and
// projects it. Returns the loaded skill and resolved source so the caller can
// update the config and lockfile.
func (inst *Installer) InstallSkill(ctx context.Context, src source.Source) (skill.Skill, *source.ResolvedSource, error) {
	resolved, err := src.Fetch(ctx, inst.Store)
	if err != nil {
		return nil, nil, fmt.Errorf("fetching skill: %w", err)
	}

	s, err := skill.Load(resolved.Dir)
	if err != nil {
		return nil, nil, fmt.Errorf("loading skill: %w", err)
	}

	if err := s.Validate(); err != nil {
		return nil, nil, fmt.Errorf("validating skill: %w", err)
	}

	if err := inst.projectSkills([]skill.Skill{s}); err != nil {
		return nil, nil, err
	}

	return s, resolved, nil
}

func (inst *Installer) projectionOpts() projector.ProjectionOpts {
	opts := projector.ProjectionOpts{ProjectDir: inst.ProjectDir}
	if inst.Global {
		opts.Scope = projector.ScopeGlobal
	}
	return opts
}

func (inst *Installer) projectSkills(skills []skill.Skill) error {
	opts := inst.projectionOpts()
	for _, agent := range inst.Agents {
		proj, ok := projector.GetProjector(agent)
		if !ok {
			return fmt.Errorf("no projector registered for agent %q", agent)
		}
		if !proj.SupportsSkills() {
			continue
		}
		if err := proj.ProjectSkills(opts, skills); err != nil {
			return fmt.Errorf("projecting skills for %s: %w", agent, err)
		}
	}
	return nil
}

func (inst *Installer) projectMCPServers(servers []mcp.MCPServer) error {
	opts := inst.projectionOpts()
	for _, agent := range inst.Agents {
		proj, ok := projector.GetProjector(agent)
		if !ok {
			return fmt.Errorf("no projector registered for agent %q", agent)
		}
		if !proj.SupportsMCPServers() {
			continue
		}
		if err := proj.ProjectMCPServers(opts, servers); err != nil {
			return fmt.Errorf("projecting MCP servers for %s: %w", agent, err)
		}
	}
	return nil
}

// InstallMCP fetches a single MCP source, loads and validates the server, and
// projects it. Returns the loaded server and resolved source so the caller can
// update the config and lockfile.
func (inst *Installer) InstallMCP(ctx context.Context, name string, src source.Source) (mcp.MCPServer, *source.ResolvedSource, error) {
	resolved, err := src.Fetch(ctx, inst.Store)
	if err != nil {
		return nil, nil, fmt.Errorf("fetching MCP server: %w", err)
	}

	server, err := mcp.Load(resolved.Dir)
	if err != nil {
		return nil, nil, fmt.Errorf("loading MCP server: %w", err)
	}

	if err := server.Validate(); err != nil {
		return nil, nil, fmt.Errorf("validating MCP server: %w", err)
	}

	if err := inst.projectMCPServers([]mcp.MCPServer{server}); err != nil {
		return nil, nil, err
	}

	return server, resolved, nil
}

// RemoveSkill removes a skill's projections from all registered agents.
func (inst *Installer) RemoveSkill(name string) error {
	opts := inst.projectionOpts()
	for _, agent := range inst.Agents {
		proj, ok := projector.GetProjector(agent)
		if !ok {
			return fmt.Errorf("no projector registered for agent %q", agent)
		}
		if !proj.SupportsSkills() {
			continue
		}
		if err := proj.UnprojectSkills(opts, []string{name}); err != nil {
			return fmt.Errorf("unprojecting skill %q for %s: %w", name, agent, err)
		}
	}
	return nil
}

// RemoveMCP removes an MCP server's projections from all registered agents.
func (inst *Installer) RemoveMCP(name string) error {
	opts := inst.projectionOpts()
	for _, agent := range inst.Agents {
		proj, ok := projector.GetProjector(agent)
		if !ok {
			return fmt.Errorf("no projector registered for agent %q", agent)
		}
		if !proj.SupportsMCPServers() {
			continue
		}
		if err := proj.UnprojectMCPServers(opts, []string{name}); err != nil {
			return fmt.Errorf("unprojecting MCP server %q for %s: %w", name, agent, err)
		}
	}
	return nil
}

func mcpLockEntryFromResolved(name string, ms config.MCPSource, resolved *source.ResolvedSource) config.MCPLockEntry {
	entry := config.MCPLockEntry{
		Name:        name,
		Transport:   ms.Transport,
		Integrity:   resolved.Integrity,
		InstallPath: resolved.Dir,
	}
	if ms.ManagedStdioMCPConfig != nil {
		entry.Package = ms.Package
	}
	if ms.UnmanagedStdioMCPConfig != nil {
		entry.Command = ms.Command
	}
	if ms.LocalMCPConfig != nil && len(ms.Args) > 0 {
		entry.Args = ms.Args
	}
	if ms.ContainerMCPConfig != nil {
		entry.Image = ms.Image
		if ms.Port != nil {
			entry.Port = *ms.Port
		}
	}
	if ms.ExternalHttpMCPConfig != nil {
		entry.URL = ms.URL
	}
	if ms.LocalMCPConfig != nil {
		entry.EnvKeys = mapKeys(ms.Env)
	}
	if ms.HttpMCPConfig != nil {
		entry.HeaderKeys = mapKeys(ms.Headers)
	}
	return entry
}

func mapKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func lockEntryFromResolved(ss config.SkillSource, resolved *source.ResolvedSource) config.SkillLockEntry {
	return config.SkillLockEntry{
		Git:       ss.Git,
		Path:      ss.Path,
		Ref:       resolved.Ref,
		Commit:    resolved.Commit,
		Integrity: resolved.Integrity,
	}
}

// buildLockIndex creates a lookup map from existing lockfile entries,
// keyed by git URL + path (for git sources) or just path (for local sources).
func buildLockIndex(lf *config.LockFile) map[string]config.SkillLockEntry {
	if lf == nil {
		return nil
	}
	idx := make(map[string]config.SkillLockEntry, len(lf.Skills))
	for _, entry := range lf.Skills {
		idx[lockKeyFromEntry(entry)] = entry
	}
	return idx
}

func lockKey(ss config.SkillSource) string {
	if ss.Git != "" {
		return ss.Git + "|" + ss.Path
	}
	return ss.Path
}

func lockKeyFromEntry(entry config.SkillLockEntry) string {
	if entry.Git != "" {
		return entry.Git + "|" + entry.Path
	}
	return entry.Path
}
