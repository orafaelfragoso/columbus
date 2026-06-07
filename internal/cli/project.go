package cli

import (
	"io"
	"log/slog"
	"path/filepath"

	"github.com/orafaelfragoso/columbus/internal/config"
	"github.com/orafaelfragoso/columbus/internal/logging"
	"github.com/orafaelfragoso/columbus/internal/store"
)

// projectContext bundles the loaded config, an open store and a project logger.
type projectContext struct {
	Config   config.Config
	DB       *store.DB
	Paths    config.Paths
	Warnings []string
	Logger   *slog.Logger

	logCloser io.Closer
}

// openProject loads .columbus.json from the work dir, opens the project store
// and attaches a per-project JSONL logger. It returns NOT_INITIALIZED when the
// project is not set up. Callers must Close.
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

	logger := logging.NewDiscard()
	var closer io.Closer
	level := logging.ParseLevel(env.Getenv("COLUMBUS_LOG_LEVEL"))
	if lg, c, lerr := logging.New(paths.LogPath, level, env.Clock, loaded.Config.ProjectID); lerr == nil {
		logger, closer = lg, c
	}

	return &projectContext{
		Config:    loaded.Config,
		DB:        db,
		Paths:     paths,
		Warnings:  loaded.Warnings,
		Logger:    logger,
		logCloser: closer,
	}, nil
}

// Close releases the store and the log file.
func (p *projectContext) Close() error {
	err := p.DB.Close()
	if p.logCloser != nil {
		p.logCloser.Close()
	}
	return err
}
