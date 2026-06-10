package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/nicolegros/skl/internal/config"
	"github.com/nicolegros/skl/internal/lock"
	"github.com/spf13/cobra"
)

func newList() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed skills",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			lockPath := filepath.Join(config.Dir(), "skills-lock.json")

			lf, err := lock.Load(lockPath)
			if err != nil {
				return err
			}

			if len(lf.Skills) == 0 {
				fmt.Println("No skills installed.")
				return nil
			}

			for _, s := range lf.Skills {
				pin := ""
				if s.Pinned {
					pin = " (pinned)"
				}
				fmt.Printf("%-20s %s@%s%s\n", s.Name, s.Repo, s.Ref[:min(7, len(s.Ref))], pin)
			}
			return nil
		},
	}
}
