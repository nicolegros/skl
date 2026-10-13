package cmd

import (
	"fmt"

	"github.com/nicolegros/skl/internal/config"
	"github.com/nicolegros/skl/internal/github"
	"github.com/nicolegros/skl/internal/skills"
)

func load() (*config.Config, skills.Env, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, skills.Env{}, err
	}
	return cfg, skills.Env{
		Paths: skills.Paths{
			Store:   config.StoreDir(),
			Backups: config.BackupsDir(),
			Dirs:    cfg.Directories,
			Lock:    config.LockPath(),
		},
		BaseURL: "https://api.github.com",
		Token:   github.Token(),
		Logf: func(format string, a ...any) {
			fmt.Printf(format+"\n", a...)
		},
	}, nil
}
