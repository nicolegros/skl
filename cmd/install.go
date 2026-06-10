package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nicolaslegros/skills/internal/config"
	"github.com/nicolaslegros/skills/internal/github"
	"github.com/nicolaslegros/skills/internal/skills"
	"github.com/spf13/cobra"
)

func newInstall() *cobra.Command {
	var ref string
	var all bool

	cmd := &cobra.Command{
		Use:   "install <owner/repo or URL> [path]",
		Short: "Install a skill from a GitHub repository",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			owner, repo, err := github.ParseRepo(args[0])
			if err != nil {
				return err
			}

			var path string
			if len(args) > 1 {
				path = args[1]
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}

			dirs := make([]string, len(cfg.Directories))
			for i, d := range cfg.Directories {
				dirs[i] = skills.ExpandPath(d)
			}

			lockPath := filepath.Join(config.Dir(), "skills-lock.json")
			token := os.Getenv("GITHUB_TOKEN")

			opts := skills.InstallOptions{
				Owner:    owner,
				Repo:     repo,
				Path:     path,
				Ref:      ref,
				Pinned:   ref != "",
				BaseURL:  "https://api.github.com",
				Dirs:     dirs,
				LockPath: lockPath,
				Token:    token,
			}

			if all {
				if err := skills.InstallAll(opts); err != nil {
					return err
				}
				fmt.Println("Installed all skills from", args[0])
			} else {
				if err := skills.Install(opts); err != nil {
					return err
				}
				name := repo
				if path != "" {
					name = filepath.Base(path)
				}
				fmt.Printf("Installed %s from %s\n", name, args[0])
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&ref, "ref", "", "Pin to a specific branch, tag, or commit SHA")
	cmd.Flags().BoolVar(&all, "all", false, "Install all skills found in the repo")
	return cmd
}
