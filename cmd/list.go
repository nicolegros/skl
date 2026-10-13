package cmd

import (
	"fmt"
	"strings"

	"github.com/nicolegros/skl/internal/skills"
	"github.com/spf13/cobra"
)

var statusFlags = map[skills.ChangeKind]string{
	skills.Add:     " (not installed)",
	skills.Changed: " (changed)",
	skills.Prune:   " (not in skl.yaml, removed on next install)",
}

func newList() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List skills in skl.yaml and their install state",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, env, err := load()
			if err != nil {
				return err
			}
			entries, err := skills.List(cfg, env.Paths)
			if err != nil {
				return err
			}

			if len(entries) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No skills installed.")
				return nil
			}

			maxName := 0
			for _, e := range entries {
				maxName = max(maxName, len(e.Name))
			}

			for _, e := range entries {
				source := e.Source.String()
				if e.Commit != "" {
					source += " " + e.Commit[:min(7, len(e.Commit))]
				}
				unlinked := ""
				if len(e.Unlinked) > 0 {
					unlinked = " (unlinked: " + strings.Join(e.Unlinked, ", ") + ")"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-*s   %s%s%s\n", maxName, e.Name, source, statusFlags[e.Status], unlinked)
			}
			return nil
		},
	}
}
