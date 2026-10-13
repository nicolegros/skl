package cmd

import (
	"fmt"

	"github.com/nicolegros/skl/internal/skills"
	"github.com/spf13/cobra"
)

func newRemove() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <skill-name>",
		Short: "Remove an installed skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := loadEnv()
			if err != nil {
				return err
			}

			if err := skills.Remove(args[0], env, resolver(false, env)); err != nil {
				return err
			}

			fmt.Printf("Removed %s\n", args[0])
			return nil
		},
	}
}
