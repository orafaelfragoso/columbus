package cli

import (
	"path/filepath"

	"github.com/rafaelfragoso/columbus/internal/config"
	"github.com/rafaelfragoso/columbus/internal/store"
)

// project bundles the loaded config and an open store for a command.
type projectContext struct {
	Config   config.Config
	DB       *store.DB
	Paths    config.Paths
	Warnings []string
}

// openProject loads .columbus.json from the work dir and opens the project
// store. It returns NOT_INITIALIZED when the project is not set up. Callers must
// Close the DB.
func (env *Env) openProject() (*projectContext, error) {
	configPath := filepath.Join(env.WorkDir, config.FileName)
	loaded, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	dataDir, err := config.DataDir(env.Getenv)
	if err != nil {
		return nil, err
	}
	paths := config.ProjectPaths(dataDir, loaded.Config.ProjectID)
	db, err := store.Open(paths.DBPath)
	if err != nil {
		return nil, err
	}
	return &projectContext{
		Config:   loaded.Config,
		DB:       db,
		Paths:    paths,
		Warnings: loaded.Warnings,
	}, nil
}
