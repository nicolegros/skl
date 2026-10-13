package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/nicolegros/skl/internal/config"
	"github.com/nicolegros/skl/internal/github"
	"github.com/nicolegros/skl/internal/skills"
)

func loadEnv() (skills.Env, error) {
	cfg, err := config.Load()
	if err != nil {
		return skills.Env{}, err
	}
	return skills.Env{
		Paths: skills.Paths{
			Store:   config.StoreDir(),
			Backups: config.BackupsDir(),
			Dirs:    cfg.Directories,
			Lock:    lockPath(),
		},
		BaseURL: "https://api.github.com",
		Token:   github.Token(),
		Logf: func(format string, a ...any) {
			fmt.Printf(format+"\n", a...)
		},
	}, nil
}

func lockPath() string {
	return filepath.Join(config.Dir(), "skl.lock")
}
