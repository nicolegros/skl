package cmd

import (
	"fmt"
	"os"

	"github.com/nicolegros/skl/internal/skills"
	"github.com/spf13/cobra"
)

func newUpdate() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "update [skill-name]",
		Short: "Update installed skills from upstream",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := loadEnv()
			if err != nil {
				return err
			}

			var name string
			if len(args) > 0 {
				name = args[0]
			}

			result, err := skills.Update(name, env, resolver(force, env))
			if result != nil {
				for _, w := range result.Warnings {
					fmt.Fprintf(os.Stderr, "warning: %s\n", w)
				}
				for _, r := range result.Results {
					if r.Skipped {
						fmt.Fprintf(os.Stderr, "Skipped %s\n", r.Name)
					} else {
						fmt.Printf("Updated %s\n", r.Name)
					}
				}
			}
			return err
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite local modifications without prompting")
	return cmd
}
