package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/nicolegros/skl/internal/config"
	"github.com/nicolegros/skl/internal/github"
	"github.com/nicolegros/skl/internal/skills"
)

func loadPaths() (skills.Paths, error) {
	cfg, err := config.Load()
	if err != nil {
		return skills.Paths{}, err
	}
	return skills.Paths{
		Store:   config.StoreDir(),
		Backups: config.BackupsDir(),
		Dirs:    cfg.Directories,
		Lock:    filepath.Join(config.Dir(), "skl.lock"),
	}, nil
}

func loadEnv() (skills.Env, error) {
	paths, err := loadPaths()
	if err != nil {
		return skills.Env{}, err
	}
	return skills.Env{
		Paths:   paths,
		BaseURL: "https://api.github.com",
		Token:   github.Token(),
		Logf: func(format string, a ...any) {
			fmt.Printf(format+"\n", a...)
		},
	}, nil
}
