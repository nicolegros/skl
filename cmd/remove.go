package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/nicolegros/skl/internal/config"
	"github.com/nicolegros/skl/internal/skills"
	"github.com/spf13/cobra"
)

func newRemove() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <skill-name>",
		Short: "Remove an installed skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			dirs := make([]string, len(cfg.Directories))
			for i, d := range cfg.Directories {
				dirs[i] = skills.ExpandPath(d)
			}

			lockPath := filepath.Join(config.Dir(), "skills-lock.json")

			if err := skills.Remove(skills.RemoveOptions{
				Name:     args[0],
				Dirs:     dirs,
				LockPath: lockPath,
			}); err != nil {
				return err
			}

			fmt.Printf("Removed %s\n", args[0])
			return nil
		},
	}
}
