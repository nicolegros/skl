package main

import (
	"os"

	"github.com/nicolegros/skl/cmd"
)

func main() {
	if err := cmd.NewRoot().Execute(); err != nil {
		os.Exit(1)
	}
}
