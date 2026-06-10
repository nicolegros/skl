package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nicolegros/skl/internal/config"
	"github.com/nicolegros/skl/internal/skills"
	"github.com/spf13/cobra"
)

func newUpdate() *cobra.Command {
	return &cobra.Command{
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
				Token:    os.Getenv("GITHUB_TOKEN"),
			})
			if err != nil {
				return err
			}

			for _, w := range result.Warnings {
				fmt.Fprintf(os.Stderr, "warning: %s\n", w)
			}
			for _, u := range result.Updated {
				fmt.Printf("Updated %s\n", u)
			}
			return nil
		},
	}
}
