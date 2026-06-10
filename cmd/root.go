package cmd

import (
	"github.com/spf13/cobra"
)

func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "skl",
		Short: "Manage agent skill installations from GitHub",
	}
	root.AddCommand(newInstall(), newUpdate(), newRemove(), newList())
	return root
}
