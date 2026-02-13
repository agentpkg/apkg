package cmd

import (
	"fmt"
	"sort"

	"github.com/agentpkg/agentpkg/pkg/config"
	"github.com/agentpkg/agentpkg/pkg/installer"
	"github.com/agentpkg/agentpkg/pkg/store"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

func newRemoveCmd() *cobra.Command {
	removeCmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove installed packages",
		Long:  "Removes skills and MCP servers from apkg.toml, the lockfile, and agent configurations.",
		RunE:  runRemoveAll,
	}

	removeCmd.Flags().Bool("all", false, "Remove all skills and MCP servers without prompting")

	skillCmd := &cobra.Command{
		Use:   "skill [name]",
		Short: "Remove a skill",
		Long:  "Removes a skill from apkg.toml, the lockfile, and agent configurations.",
		Args:  cobra.ExactArgs(1),
		RunE:  runRemoveSkill,
	}

	mcpCmd := &cobra.Command{
		Use:   "mcp [name]",
		Short: "Remove an MCP server",
		Long:  "Removes an MCP server from apkg.toml, the lockfile, and agent configurations.",
		Args:  cobra.ExactArgs(1),
		RunE:  runRemoveMCP,
	}

	removeCmd.AddCommand(skillCmd)
	removeCmd.AddCommand(mcpCmd)
	return removeCmd
}

func runRemoveAll(cmd *cobra.Command, args []string) error {
	global, err := cmd.Flags().GetBool("global")
	if err != nil {
		return err
	}

	projectDir, manifestPath, lockPath, err := resolveInstallPaths(global)
	if err != nil {
		return err
	}

	cfg, err := config.LoadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("loading %s: %w", manifestPath, err)
	}

	if len(cfg.Skills) == 0 && len(cfg.MCPServers) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Nothing to remove")
		return nil
	}

	all, err := cmd.Flags().GetBool("all")
	if err != nil {
		return err
	}

	// Build sorted lists of names for deterministic ordering.
	skillNames := sortedKeys(cfg.Skills)
	mcpNames := sortedKeys(cfg.MCPServers)

	var selectedSkills, selectedMCPs []string

	if all {
		selectedSkills = skillNames
		selectedMCPs = mcpNames
	} else {
		// Build options with prefixed labels so the user can distinguish types.
		type entry struct {
			label string
			kind  string // "skill" or "mcp"
			name  string
		}

		var entries []entry
		for _, name := range skillNames {
			entries = append(entries, entry{label: "skill: " + name, kind: "skill", name: name})
		}
		for _, name := range mcpNames {
			entries = append(entries, entry{label: "mcp: " + name, kind: "mcp", name: name})
		}

		options := make([]huh.Option[int], len(entries))
		for i, e := range entries {
			options[i] = huh.NewOption(e.label, i)
		}

		var selectedIdxs []int
		err := huh.NewForm(
			huh.NewGroup(
				huh.NewMultiSelect[int]().
					Title("Select packages to remove").
					Options(options...).
					Value(&selectedIdxs),
			),
		).Run()
		if err != nil {
			return fmt.Errorf("selection prompt failed: %w", err)
		}

		if len(selectedIdxs) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "Nothing selected")
			return nil
		}

		for _, idx := range selectedIdxs {
			e := entries[idx]
			if e.kind == "skill" {
				selectedSkills = append(selectedSkills, e.name)
			} else {
				selectedMCPs = append(selectedMCPs, e.name)
			}
		}
	}

	agents, err := resolveAgents(global)
	if err != nil {
		return err
	}

	s, err := store.Default()
	if err != nil {
		return err
	}

	inst := &installer.Installer{
		Store:      s,
		ProjectDir: projectDir,
		Agents:     agents,
		Global:     global,
	}

	for _, name := range selectedSkills {
		if err := inst.RemoveSkill(name); err != nil {
			return err
		}
		delete(cfg.Skills, name)
	}

	for _, name := range selectedMCPs {
		if err := inst.RemoveMCP(name); err != nil {
			return err
		}
		delete(cfg.MCPServers, name)
	}

	if err := config.SaveFile(manifestPath, cfg); err != nil {
		return fmt.Errorf("saving %s: %w", manifestPath, err)
	}

	lf, err := config.LoadLockFile(lockPath)
	if err != nil {
		return fmt.Errorf("loading lockfile: %w", err)
	}

	lf.Skills = filterSkillLockEntries(lf.Skills, selectedSkills)
	lf.MCPServers = filterMCPLockEntries(lf.MCPServers, selectedMCPs)

	if err := config.SaveLockFile(lockPath, lf); err != nil {
		return fmt.Errorf("writing lockfile: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Removed %d skill(s) and %d MCP server(s)\n", len(selectedSkills), len(selectedMCPs))
	return nil
}

func runRemoveSkill(cmd *cobra.Command, args []string) error {
	global, err := cmd.Flags().GetBool("global")
	if err != nil {
		return err
	}

	projectDir, manifestPath, lockPath, err := resolveInstallPaths(global)
	if err != nil {
		return err
	}

	name := args[0]

	cfg, err := config.LoadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("loading %s: %w", manifestPath, err)
	}

	if _, ok := cfg.Skills[name]; !ok {
		return fmt.Errorf("skill %q not found in %s", name, manifestPath)
	}

	agents, err := resolveAgents(global)
	if err != nil {
		return err
	}

	s, err := store.Default()
	if err != nil {
		return err
	}

	inst := &installer.Installer{
		Store:      s,
		ProjectDir: projectDir,
		Agents:     agents,
		Global:     global,
	}

	if err := inst.RemoveSkill(name); err != nil {
		return err
	}

	delete(cfg.Skills, name)
	if err := config.SaveFile(manifestPath, cfg); err != nil {
		return fmt.Errorf("saving %s: %w", manifestPath, err)
	}

	lf, err := config.LoadLockFile(lockPath)
	if err != nil {
		return fmt.Errorf("loading lockfile: %w", err)
	}

	lf.Skills = filterSkillLockEntries(lf.Skills, []string{name})

	if err := config.SaveLockFile(lockPath, lf); err != nil {
		return fmt.Errorf("writing lockfile: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Removed skill %q\n", name)
	return nil
}

func runRemoveMCP(cmd *cobra.Command, args []string) error {
	global, err := cmd.Flags().GetBool("global")
	if err != nil {
		return err
	}

	projectDir, manifestPath, lockPath, err := resolveInstallPaths(global)
	if err != nil {
		return err
	}

	name := args[0]

	cfg, err := config.LoadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("loading %s: %w", manifestPath, err)
	}

	if _, ok := cfg.MCPServers[name]; !ok {
		return fmt.Errorf("MCP server %q not found in %s", name, manifestPath)
	}

	agents, err := resolveAgents(global)
	if err != nil {
		return err
	}

	s, err := store.Default()
	if err != nil {
		return err
	}

	inst := &installer.Installer{
		Store:      s,
		ProjectDir: projectDir,
		Agents:     agents,
		Global:     global,
	}

	if err := inst.RemoveMCP(name); err != nil {
		return err
	}

	delete(cfg.MCPServers, name)
	if err := config.SaveFile(manifestPath, cfg); err != nil {
		return fmt.Errorf("saving %s: %w", manifestPath, err)
	}

	lf, err := config.LoadLockFile(lockPath)
	if err != nil {
		return fmt.Errorf("loading lockfile: %w", err)
	}

	lf.MCPServers = filterMCPLockEntries(lf.MCPServers, []string{name})

	if err := config.SaveLockFile(lockPath, lf); err != nil {
		return fmt.Errorf("writing lockfile: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Removed MCP server %q\n", name)
	return nil
}

// filterSkillLockEntries returns entries with the given names removed.
func filterSkillLockEntries(entries []config.SkillLockEntry, names []string) []config.SkillLockEntry {
	remove := make(map[string]bool, len(names))
	for _, n := range names {
		remove[n] = true
	}
	var kept []config.SkillLockEntry
	for _, e := range entries {
		if !remove[e.Name] {
			kept = append(kept, e)
		}
	}
	return kept
}

// filterMCPLockEntries returns entries with the given names removed.
func filterMCPLockEntries(entries []config.MCPLockEntry, names []string) []config.MCPLockEntry {
	remove := make(map[string]bool, len(names))
	for _, n := range names {
		remove[n] = true
	}
	var kept []config.MCPLockEntry
	for _, e := range entries {
		if !remove[e.Name] {
			kept = append(kept, e)
		}
	}
	return kept
}

// sortedKeys returns the keys of a map sorted alphabetically.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
