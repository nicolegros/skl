package cmd

import (
	"cmp"
	"fmt"

	"github.com/nicolegros/skl/internal/config"
	"github.com/nicolegros/skl/internal/skills"
	"github.com/spf13/cobra"
)

func newInstall() *cobra.Command {
	var ref string
	var all bool
	var as string
	var force bool

	cmd := &cobra.Command{
		Use:     "install [owner/repo[/path][@ref] or URL] [path]",
		Aliases: []string{"i"},
		Short:   "Install a skill and add it to skl.yaml, or install everything skl.yaml lists",
		Args:    cobra.RangeArgs(0, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if as != "" && all {
				return fmt.Errorf("--as and --all are mutually exclusive")
			}
			if len(args) == 0 && (as != "" || all || ref != "") {
				return fmt.Errorf("--as, --all, and --ref need a source")
			}

			cfg, env, err := load()
			if err != nil {
				return err
			}
			resolve := resolver(force, env)

			if len(args) == 0 {
				results, err := skills.Converge(cfg, env, resolve)
				return report(cmd, results, err)
			}

			src, err := sourceFromArgs(args, ref)
			if err != nil {
				return err
			}
			wanted := map[string]config.Source{cmp.Or(as, src.Name()): src}
			if all {
				if wanted, err = skills.DiscoverAll(src, env); err != nil {
					return err
				}
			}
			results, err := skills.Install(cfg, wanted, env, resolve)
			return report(cmd, results, err)
		},
	}

	cmd.Flags().StringVar(&ref, "ref", "", "Branch, tag, or commit SHA to install (default branch if empty)")
	cmd.Flags().BoolVar(&all, "all", false, "Install all skills found in the repo")
	cmd.Flags().StringVar(&as, "as", "", "Install the skill under a different name")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite local modifications without prompting")
	return cmd
}

// sourceFromArgs combines a source string, an optional path argument, and --ref.
func sourceFromArgs(args []string, ref string) (config.Source, error) {
	src, err := config.ParseSource(args[0])
	if err != nil {
		return src, err
	}
	if len(args) > 1 {
		if src.Path != "" {
			return src, fmt.Errorf("path given twice: %s and %s", src.Path, args[1])
		}
		withPath, err := config.ParseSource(src.Repo + "/" + args[1])
		if err != nil {
			return src, err
		}
		src.Path = withPath.Path
		if withPath.Ref != "" && src.Ref != "" {
			return src, fmt.Errorf("ref given twice: @%s and @%s", src.Ref, withPath.Ref)
		}
		src.Ref = cmp.Or(src.Ref, withPath.Ref)
	}
	if ref != "" {
		if src.Ref != "" && src.Ref != ref {
			return src, fmt.Errorf("--ref %s conflicts with @%s", ref, src.Ref)
		}
		src.Ref = ref
	}
	return src, nil
}
