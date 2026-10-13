package cmd

import (
	"github.com/nicolegros/skl/internal/skills"
	"github.com/spf13/cobra"
)

func newRemove() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <skill-name>",
		Short: "Remove a skill from skl.yaml and uninstall it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, env, err := load()
			if err != nil {
				return err
			}
			results, err := skills.Remove(cfg, args[0], env, resolver(false, env))
			return report(cmd, results, err)
		},
	}
}
