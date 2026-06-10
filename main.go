package main

import (
	"os"

	"github.com/nicolaslegros/skills/cmd"
)

func main() {
	if err := cmd.NewRoot().Execute(); err != nil {
		os.Exit(1)
	}
}
