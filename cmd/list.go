package cmd

import (
	"fmt"
	"strings"

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
			paths, err := loadPaths()
			if err != nil {
				return err
			}
			entries, err := skills.List(paths)
			if err != nil {
				return err
			}

			if len(entries) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No skills installed.")
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
				unlinked := ""
				if len(e.Unlinked) > 0 {
					unlinked = " (unlinked: " + strings.Join(e.Unlinked, ", ") + ")"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-*s   %s@%s%s\n", maxName, e.DisplayName, e.Source, e.Commit[:min(7, len(e.Commit))], unlinked)
			}
			return nil
		},
	}
}
