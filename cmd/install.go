package cmd

import (
	"fmt"
	"os"

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
		Short:   "Install a skill from a GitHub repository, or link every skill from the lock file",
		Args:    cobra.RangeArgs(0, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if as != "" && all {
				return fmt.Errorf("--as and --all are mutually exclusive")
			}

			env, err := loadEnv()
			if err != nil {
				return err
			}
			resolve := resolver(force, env)

			if len(args) == 0 {
				return skills.Reconcile(env, resolve)
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
				Owner: owner,
				Repo:  repo,
				Path:  path,
				Ref:   ref,
				Alias: as,
			}

			var results []skills.Result
			if all {
				results, err = skills.InstallAll(opts, env, resolve)
			} else {
				var result skills.Result
				result, err = skills.Install(opts, env, resolve)
				results = []skills.Result{result}
			}
			for _, r := range results {
				if r.Skipped {
					fmt.Fprintf(os.Stderr, "Skipped %s\n", r.Name)
				} else if r.Name != "" {
					fmt.Printf("Installed %s from %s\n", r.Name, args[0])
				}
			}
			return err
		},
	}

	cmd.Flags().StringVar(&ref, "ref", "", "Pin to a specific branch, tag, or commit SHA")
	cmd.Flags().BoolVar(&all, "all", false, "Install all skills found in the repo")
	cmd.Flags().StringVar(&as, "as", "", "Install the skill under a different name")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite local modifications without prompting")
	return cmd
}
