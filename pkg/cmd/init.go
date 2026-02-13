package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/agentpkg/agentpkg/pkg/config"
	"github.com/agentpkg/agentpkg/pkg/project"
	"github.com/agentpkg/agentpkg/pkg/projector"
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

	// Prompt for agent config directories to gitignore.
	selectedEntries, err := promptGitignoreEntries()
	if err != nil {
		return err
	}

	gitignoreEntries := append([]string{config.LocalConfigFile}, selectedEntries...)
	added, err := project.EnsureGitignore(wd, gitignoreEntries)
	if err != nil {
		return err
	}
	for _, entry := range added {
		fmt.Fprintf(cmd.OutOrStdout(), "Added %s to .gitignore\n", entry)
	}

	return nil
}

// promptGitignoreEntries uses huh to present a multi-select of agent config
// entries to gitignore, built from the registered projectors.
func promptGitignoreEntries() ([]string, error) {
	agents := projector.RegisteredAgents()
	if len(agents) == 0 {
		return nil, nil
	}

	type agentOption struct {
		label   string
		entries []string
	}

	opts := make([]agentOption, 0, len(agents))
	for _, agent := range agents {
		proj, _ := projector.GetProjector(agent)
		entries := proj.GitignoreEntries()
		if len(entries) == 0 {
			continue
		}
		label := fmt.Sprintf("%s config files [%s]", agent, strings.Join(entries, ", "))
		opts = append(opts, agentOption{label: label, entries: entries})
	}

	if len(opts) == 0 {
		return nil, nil
	}

	options := make([]huh.Option[int], len(opts))
	for i, opt := range opts {
		options[i] = huh.NewOption(opt.label, i)
	}

	var selected []int
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[int]().
				Title("Add agent config files to .gitignore?").
				Options(options...).
				Value(&selected),
		),
	).Run()
	if err != nil {
		return nil, fmt.Errorf("prompt failed: %w", err)
	}

	var entries []string
	for _, idx := range selected {
		entries = append(entries, opts[idx].entries...)
	}
	return entries, nil
}
