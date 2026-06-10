package main

import (
	"os"

	"github.com/nicolegros/skl/cmd"
)

var version = "dev"

func main() {
	if err := cmd.NewRoot(version).Execute(); err != nil {
		os.Exit(1)
	}
}
