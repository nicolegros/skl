package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewRoot(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "skl",
		Short: "Manage agent skill installations from GitHub",
	}
	root.AddCommand(newInstall(), newUpdate(), newRemove(), newList())
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run:   func(cmd *cobra.Command, args []string) { fmt.Println(version) },
	})
	return root
}
