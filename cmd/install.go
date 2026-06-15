package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/nicolegros/skl/internal/config"
	"github.com/nicolegros/skl/internal/github"
	"github.com/nicolegros/skl/internal/skills"
	"github.com/spf13/cobra"
)

func newInstall() *cobra.Command {
	var ref string
	var all bool

	cmd := &cobra.Command{
		Use:     "install [owner/repo or URL] [path]",
		Aliases: []string{"i"},
		Short:   "Install a skill from a GitHub repository, or all missing skills from lock file",
		Args:    cobra.RangeArgs(0, 2),
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
			token := github.Token()

			if len(args) == 0 {
				return skills.InstallFromLock(lockPath, "https://api.github.com", token, dirs, func(format string, a ...any) {
					fmt.Printf(format+"\n", a...)
				})
			}

			owner, repo, err := github.ParseRepo(args[0])
			if err != nil {
				return err
			}

			var path string
			if len(args) > 1 {
				path = args[1]
			}

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
				names, err := skills.InstallAll(opts)
				if err != nil {
					return err
				}
				for _, name := range names {
					fmt.Printf("Installed %s from %s\n", name, args[0])
				}
			} else {
				name, err := skills.Install(opts)
				if err != nil {
					return err
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
