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

func newInstall() *cobra.Command {
	var ref string
	var all bool
	var as string
	var force bool

	cmd := &cobra.Command{
		Use:     "install [owner/repo or URL] [path]",
		Aliases: []string{"i"},
		Short:   "Install a skill from a GitHub repository, or all missing skills from lock file",
		Args:    cobra.RangeArgs(0, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if as != "" && all {
				return fmt.Errorf("--as and --all are mutually exclusive")
			}

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
				Alias:    as,
				BaseURL:  "https://api.github.com",
				Dirs:     dirs,
				LockPath: lockPath,
				Token:    token,
				Logf: func(format string, a ...any) {
					fmt.Printf(format+"\n", a...)
				},
				Force:    force,
			}

			if all {
				result, err := skills.InstallAll(opts)
				if err != nil {
					return err
				}
				for _, name := range result.Installed {
					fmt.Printf("Installed %s from %s\n", name, args[0])
				}
				for _, skipped := range result.Skipped {
					if promptForModifications(skipped.Name, skipped.Modifications, promptContext{
						BaseURL:  "https://api.github.com",
						Token:    token,
						LockPath: lockPath,
					}) {
						singleOpts := opts
						singleOpts.Path = skipped.Path
						if singleOpts.Path == "." {
							singleOpts.Path = ""
						}
						singleOpts.Force = true
						singleResult, err := skills.Install(singleOpts)
						if err != nil {
							return err
						}
						fmt.Printf("Installed %s from %s\n", singleResult.Name, args[0])
					} else {
						fmt.Fprintf(os.Stderr, "Skipped %s\n", skipped.Name)
					}
				}
			} else {
				result, err := skills.Install(opts)
				if err != nil {
					return err
				}
				if result.Name == "" && len(result.Modifications) > 0 {
					skillName := opts.Repo
					if opts.Path != "" {
						skillName = filepath.Base(opts.Path)
					}
					if opts.Alias != "" {
						skillName = opts.Alias
					}
					if promptForModifications(skillName, result.Modifications, promptContext{
						BaseURL:  "https://api.github.com",
						Token:    token,
						LockPath: lockPath,
					}) {
						opts.Force = true
						result, err = skills.Install(opts)
						if err != nil {
							return err
						}
						fmt.Printf("Installed %s from %s\n", result.Name, args[0])
					} else {
						fmt.Fprintf(os.Stderr, "Skipped\n")
					}
					return nil
				}
				fmt.Printf("Installed %s from %s\n", result.Name, args[0])
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&ref, "ref", "", "Pin to a specific branch, tag, or commit SHA")
	cmd.Flags().BoolVar(&all, "all", false, "Install all skills found in the repo")
	cmd.Flags().StringVar(&as, "as", "", "Install the skill under a different name")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite local modifications without prompting")
	return cmd
}
