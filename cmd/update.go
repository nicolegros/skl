package cmd

import (
	"github.com/nicolegros/skl/internal/skills"
	"github.com/spf13/cobra"
)

func newUpdate() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "update [skill-name]",
		Short: "Move installed skills to the latest commit of their declared ref",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, env, err := load()
			if err != nil {
				return err
			}
			var name string
			if len(args) > 0 {
				name = args[0]
			}
			results, err := skills.Update(cfg, name, env, resolver(force, env))
			return report(cmd, results, err)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite local modifications without prompting")
	return cmd
}
