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

			// Compute dynamic column width from longest name.
			maxName := 0
			for _, e := range entries {
				if len(e.DisplayName) > maxName {
					maxName = len(e.DisplayName)
				}
			}

			for _, e := range entries {
				pin := ""
				if e.Pinned {
					pin = " (pinned)"
				}
				fmt.Printf("%-*s   %s@%s%s\n", maxName, e.DisplayName, e.Source, e.Ref[:min(7, len(e.Ref))], pin)
			}
			return nil
		},
	}
}
