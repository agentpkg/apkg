package installer

import (
	"context"
	"fmt"
	"sort"

	"github.com/agentpkg/agentpkg/pkg/config"
	"github.com/agentpkg/agentpkg/pkg/projector"
	"github.com/agentpkg/agentpkg/pkg/skill"
	"github.com/agentpkg/agentpkg/pkg/source"
	"github.com/agentpkg/agentpkg/pkg/store"
)

type Installer struct {
	Store      store.Store
	ProjectDir string
	Agents     []string
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

func (inst *Installer) projectSkills(skills []skill.Skill) error {
	for _, agent := range inst.Agents {
		proj, ok := projector.GetProjector(agent)
		if !ok {
			return fmt.Errorf("no projector registered for agent %q", agent)
		}
		if !proj.SupportsSkills() {
			continue
		}
		if err := proj.ProjectSkills(inst.ProjectDir, skills); err != nil {
			return fmt.Errorf("projecting skills for %s: %w", agent, err)
		}
	}
	return nil
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
