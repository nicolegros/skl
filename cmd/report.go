package cmd

import (
	"fmt"

	"github.com/nicolegros/skl/internal/skills"
	"github.com/spf13/cobra"
)

// report prints what a command did to each skill and fails if any skill failed.
func report(cmd *cobra.Command, results []skills.Result, err error) error {
	failed := 0
	for _, r := range results {
		switch {
		case r.Err != nil:
			failed++
			fmt.Fprintf(cmd.ErrOrStderr(), "error: %s: %s: %v\n", r.Name, r.Source, r.Err)
		case r.Skipped:
			fmt.Fprintf(cmd.ErrOrStderr(), "Skipped %s\n", r.Name)
		case r.Action == skills.Installed:
			fmt.Fprintf(cmd.OutOrStdout(), "Installed %s from %s\n", r.Name, r.Source)
		case r.Action == skills.Updated:
			fmt.Fprintf(cmd.OutOrStdout(), "Updated %s\n", r.Name)
		case r.Action == skills.Removed:
			fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", r.Name)
		}
	}
	if err != nil {
		return err
	}
	if failed == 1 {
		return fmt.Errorf("1 skill failed")
	}
	if failed > 1 {
		return fmt.Errorf("%d skills failed", failed)
	}
	return nil
}
