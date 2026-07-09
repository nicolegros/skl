package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nicolegros/skl/internal/config"
	"github.com/nicolegros/skl/internal/github"
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
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			dirs := make([]string, len(cfg.Directories))
			for i, d := range cfg.Directories {
				dirs[i] = skills.ExpandPath(d)
			}

			lockPath := filepath.Join(config.Dir(), "skl.lock")

			var name string
			if len(args) > 0 {
				name = args[0]
			}

			result, err := skills.Update(skills.UpdateOptions{
				Name:     name,
				BaseURL:  "https://api.github.com",
				Dirs:     dirs,
				LockPath: lockPath,
				Token:    github.Token(),
				Force:    force,
			})
			if err != nil {
				return err
			}

			for _, w := range result.Warnings {
				fmt.Fprintf(os.Stderr, "warning: %s\n", w)
			}
			for _, m := range result.Modifications {
				if promptForModifications(m.SkillName, m.Dirs, promptContext{
					BaseURL:  "https://api.github.com",
					Token:    github.Token(),
					LockPath: lockPath,
				}) {
					// User chose to proceed — re-run update with force for this skill
					_, err := skills.Update(skills.UpdateOptions{
						Name:     m.SkillName,
						BaseURL:  "https://api.github.com",
						Dirs:     dirs,
						LockPath: lockPath,
						Token:    github.Token(),
						Force:    true,
					})
					if err != nil {
						fmt.Fprintf(os.Stderr, "error updating %s: %v\n", m.SkillName, err)
					} else {
						fmt.Printf("Updated %s\n", m.SkillName)
					}
				} else {
					fmt.Fprintf(os.Stderr, "Skipped %s\n", m.SkillName)
				}
			}
			for _, u := range result.Updated {
				fmt.Printf("Updated %s\n", u)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite local modifications without prompting")
	return cmd
}
