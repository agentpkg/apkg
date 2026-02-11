package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentpkg/agentpkg/pkg/config"
	"github.com/agentpkg/agentpkg/pkg/installer"
	"github.com/agentpkg/agentpkg/pkg/project"
	"github.com/agentpkg/agentpkg/pkg/projector"
	"github.com/agentpkg/agentpkg/pkg/source"
	"github.com/agentpkg/agentpkg/pkg/store"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

func newInstallCmd() *cobra.Command {
	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Install packages from apkg.toml",
		Long:  "Resolves and installs all skills listed in apkg.toml, then projects them into agent configurations.",
		RunE:  runInstallAll,
	}

	skillCmd := &cobra.Command{
		Use:   "skill [ref]",
		Short: "Add and install a skill",
		Long: `Adds a skill to apkg.toml and installs it.

A ref like owner/repo/path@ref installs from git (GitHub).
A local path starting with ./ or ../ installs from the filesystem.`,
		Args: cobra.ExactArgs(1),
		RunE: runInstallSkill,
	}

	installCmd.AddCommand(skillCmd)
	return installCmd
}

// resolveInstallPaths returns the projectDir, manifestPath, and lockPath
// based on whether the install is global or project-local.
func resolveInstallPaths(global bool) (projectDir, manifestPath, lockPath string, err error) {
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", "", fmt.Errorf("determining home directory: %w", err)
		}
		projectDir = home

		manifestPath, err = config.GlobalManifestPath()
		if err != nil {
			return "", "", "", err
		}

		lockPath, err = config.GlobalLockFilePath()
		if err != nil {
			return "", "", "", err
		}

		return projectDir, manifestPath, lockPath, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", "", "", fmt.Errorf("getting working directory: %w", err)
	}

	return wd, filepath.Join(wd, project.ManifestFile), filepath.Join(wd, config.LockFileName), nil
}

func runInstallAll(cmd *cobra.Command, args []string) error {
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

	s, err := store.Default()
	if err != nil {
		return err
	}

	existingLock, err := config.LoadLockFile(lockPath)
	if err != nil {
		return fmt.Errorf("loading lockfile: %w", err)
	}

	agents, err := resolveAgents(global)
	if err != nil {
		return err
	}

	inst := &installer.Installer{
		Store:      s,
		ProjectDir: projectDir,
		Agents:     agents,
	}

	lf, err := inst.InstallAll(cmd.Context(), cfg, existingLock)
	if err != nil {
		return err
	}

	if err := config.SaveLockFile(lockPath, lf); err != nil {
		return fmt.Errorf("writing lockfile: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Installed %d skill(s)\n", len(lf.Skills))
	if len(agents) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Warning: no agents selected, skills were not projected into any agent configuration")
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Projected %d skill(s) to %s\n", len(cfg.Skills), strings.Join(agents, ", "))
	}
	return nil
}

func runInstallSkill(cmd *cobra.Command, args []string) error {
	global, err := cmd.Flags().GetBool("global")
	if err != nil {
		return err
	}

	projectDir, manifestPath, lockPath, err := resolveInstallPaths(global)
	if err != nil {
		return err
	}

	src, skillSource, err := source.ParseRef(args[0])
	if err != nil {
		return err
	}

	s, err := store.Default()
	if err != nil {
		return err
	}

	agents, err := resolveAgents(global)
	if err != nil {
		return err
	}

	inst := &installer.Installer{
		Store:      s,
		ProjectDir: projectDir,
		Agents:     agents,
	}

	sk, resolved, err := inst.InstallSkill(cmd.Context(), src)
	if err != nil {
		return err
	}

	// Ensure global manifest exists when installing globally.
	if global {
		if err := project.InitGlobal(); err != nil {
			return err
		}
	}

	// Update apkg.toml with the new skill.
	cfg, err := config.LoadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("loading %s: %w", manifestPath, err)
	}

	if cfg.Skills == nil {
		cfg.Skills = make(map[string]config.SkillSource)
	}
	cfg.Skills[sk.Name()] = skillSource

	if err := config.SaveFile(manifestPath, cfg); err != nil {
		return fmt.Errorf("saving %s: %w", manifestPath, err)
	}

	// Update lockfile.
	lf, err := config.LoadLockFile(lockPath)
	if err != nil {
		return fmt.Errorf("loading lockfile: %w", err)
	}

	lockEntry := config.SkillLockEntry{
		Git:       skillSource.Git,
		Path:      skillSource.Path,
		Ref:       resolved.Ref,
		Commit:    resolved.Commit,
		Integrity: resolved.Integrity,
	}

	lf.Skills = upsertLockEntry(lf.Skills, lockEntry)

	if err := config.SaveLockFile(lockPath, lf); err != nil {
		return fmt.Errorf("writing lockfile: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Installed skill %q\n", sk.Name())
	if len(agents) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Warning: no agents selected, skill was not projected into any agent configuration")
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Projected 1 skill(s) to %s\n", strings.Join(agents, ", "))
	}
	return nil
}

// upsertLockEntry adds or replaces a lock entry, matching on git+path
// (for git sources) or path alone (for local sources).
func upsertLockEntry(entries []config.SkillLockEntry, entry config.SkillLockEntry) []config.SkillLockEntry {
	key := entryKey(entry)
	for i, e := range entries {
		if entryKey(e) == key {
			entries[i] = entry
			return entries
		}
	}
	return append(entries, entry)
}

func entryKey(e config.SkillLockEntry) string {
	if e.Git != "" {
		return e.Git + "|" + e.Path
	}
	return e.Path
}

// resolveAgents returns the agent list from DevCfg, or prompts the user
// to select from all registered projector agents if none are configured.
func resolveAgents(global bool) ([]string, error) {
	if len(DevCfg.Agents) > 0 {
		return DevCfg.Agents, nil
	}
	return promptAgents(global)
}

// promptAgents uses huh to present a multi-select of all registered agents,
// then asks whether to save the choice for future installs.
// When global is true, the save prompt only offers "globally" (not "for this project").
func promptAgents(global bool) ([]string, error) {
	agents := projector.RegisteredAgents()
	options := make([]huh.Option[string], len(agents))
	for i, a := range agents {
		options[i] = huh.NewOption(a, a)
	}

	var selected []string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select agents to project skills for").
				Options(options...).
				Value(&selected),
		),
	).Run()
	if err != nil {
		return nil, fmt.Errorf("agent selection prompt failed: %w", err)
	}

	if len(selected) == 0 {
		return selected, nil
	}

	var saveOptions []huh.Option[string]
	if global {
		saveOptions = []huh.Option[string]{
			huh.NewOption("Yes, globally", "global"),
			huh.NewOption("No", "no"),
		}
	} else {
		saveOptions = []huh.Option[string]{
			huh.NewOption("Yes, for this project", "project"),
			huh.NewOption("Yes, globally", "global"),
			huh.NewOption("No", "no"),
		}
	}

	var saveChoice string
	err = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Save agent selection for future installs?").
				Options(saveOptions...).
				Value(&saveChoice),
		),
	).Run()
	if err != nil {
		return nil, fmt.Errorf("save preference prompt failed: %w", err)
	}

	devCfg := &config.DevConfig{Agents: selected}
	switch saveChoice {
	case "project":
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("getting working directory: %w", err)
		}
		if err := config.WriteLocalDevConfig(wd, devCfg); err != nil {
			return nil, err
		}
	case "global":
		if err := config.WriteGlobalDevConfig(devCfg); err != nil {
			return nil, err
		}
	}

	return selected, nil
}
