package cmd

import (
	"os"

	"github.com/agentpkg/agentpkg/pkg/config"
	"github.com/spf13/cobra"
)

var (
	flagAgents []string

	// DevCfg holds the resolved developer configuration, available to all
	// subcommands after PersistentPreRunE completes.
	DevCfg *config.DevConfig
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "apkg",
		Short: "Agent package manager",
		Long:  "apkg manages agent-agnostic skill packages and projects them into coding agent configurations.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			global, _ := cmd.Flags().GetBool("global")
			cfg, err := config.LoadDevConfig(flagAgents, global)
			if err != nil {
				return err
			}
			DevCfg = cfg
			return nil
		},
		SilenceUsage: true,
	}

	root.PersistentFlags().BoolP("global", "g", false, "Install globally (~/.apkg/) instead of in the current project")
	root.PersistentFlags().StringSliceVar(&flagAgents, "agents", nil, "coding agents to project for (e.g. claude-code,cursor)")

	root.AddCommand(newInitCmd())
	root.AddCommand(newInstallCmd())

	return root
}

func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
