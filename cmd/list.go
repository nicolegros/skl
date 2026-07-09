package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/nicolegros/skl/internal/config"
	"github.com/nicolegros/skl/internal/skills"
	"github.com/spf13/cobra"
)

func newList() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List installed skills",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			lockPath := filepath.Join(config.Dir(), "skl.lock")

			entries, err := skills.List(skills.ListOptions{LockPath: lockPath})
			if err != nil {
				return err
			}

			if len(entries) == 0 {
				fmt.Println("No skills installed.")
				return nil
			}

			for _, e := range entries {
				pin := ""
				if e.Pinned {
					pin = " (pinned)"
				}
				fmt.Printf("%-20s %s@%s%s\n", e.DisplayName, e.Source, e.Ref[:min(7, len(e.Ref))], pin)
			}
			return nil
		},
	}
}
