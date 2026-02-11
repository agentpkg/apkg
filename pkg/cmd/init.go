package cmd

import (
	"fmt"
	"os"

	"github.com/agentpkg/agentpkg/pkg/config"
	"github.com/agentpkg/agentpkg/pkg/project"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a new apkg project",
		Long:  "Creates an apkg.toml manifest and configures .gitignore entries.",
		RunE:  runInit,
		// init does not need dev config resolution; skip the root PersistentPreRunE.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
}

func runInit(cmd *cobra.Command, args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	name := project.InferName(wd)

	if err := project.Init(wd, name); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", project.ManifestFile)

	// Prompt for agent directories to gitignore.
	selectedDirs, err := promptAgentDirs()
	if err != nil {
		return err
	}

	gitignoreEntries := append([]string{config.LocalConfigFile}, selectedDirs...)
	added, err := project.EnsureGitignore(wd, gitignoreEntries)
	if err != nil {
		return err
	}
	for _, entry := range added {
		fmt.Fprintf(cmd.OutOrStdout(), "Added %s to .gitignore\n", entry)
	}

	return nil
}

// promptAgentDirs uses huh to present a multi-select of agent directories.
func promptAgentDirs() ([]string, error) {
	options := make([]huh.Option[string], len(project.AgentDirs))
	for i, dir := range project.AgentDirs {
		options[i] = huh.NewOption(dir, dir)
	}

	var selected []string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Add agent directories to .gitignore?").
				Options(options...).
				Value(&selected),
		),
	).Run()
	if err != nil {
		return nil, fmt.Errorf("prompt failed: %w", err)
	}

	return selected, nil
}
