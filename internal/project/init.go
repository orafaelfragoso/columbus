// Package project owns project identity and lifecycle operations: init,
// git anchoring, and (later) file-set selection.
package project

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rafaelfragoso/columbus/internal/config"
	"github.com/rafaelfragoso/columbus/internal/contract"
	"github.com/rafaelfragoso/columbus/internal/gitrepo"
	"github.com/rafaelfragoso/columbus/internal/ids"
	"github.com/rafaelfragoso/columbus/internal/render"
	"github.com/rafaelfragoso/columbus/internal/store"
)

// InitParams are the inputs to Init.
type InitParams struct {
	WorkDir string
	IDs     ids.Source
	Getenv  func(string) string
	Ctx     context.Context
}

// InitResult is the typed result of init.
type InitResult struct {
	ProjectID          string   `json:"project_id"`
	ConfigPath         string   `json:"config_path"`
	DataDir            string   `json:"data_dir"`
	DBPath             string   `json:"db_path"`
	GitExcluded        bool     `json:"git_excluded"`
	AlreadyInitialized bool     `json:"already_initialized"`
	Warnings           []string `json:"warnings,omitempty"`
}

func (InitResult) CommandName() string { return "init" }

func (r InitResult) RenderText(w io.Writer, _ render.Options) error {
	if r.AlreadyInitialized {
		fmt.Fprintf(w, "Already initialized (project %s)\n", r.ProjectID)
	} else {
		fmt.Fprintf(w, "Initialized columbus project %s\n", r.ProjectID)
	}
	fmt.Fprintf(w, "  config:   %s", r.ConfigPath)
	if r.GitExcluded {
		fmt.Fprint(w, " (git-excluded locally)")
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  data dir: %s\n", r.DataDir)
	for _, warn := range r.Warnings {
		fmt.Fprintf(w, "  warning:  %s\n", warn)
	}
	return nil
}

func (r InitResult) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# columbus init\n\n- project_id: %s\n- config: %s\n- data_dir: %s\n- git_excluded: %t\n- already_initialized: %t\n",
		r.ProjectID, r.ConfigPath, r.DataDir, r.GitExcluded, r.AlreadyInitialized)
	return nil
}

// Init creates (or detects an existing) Columbus project: it mints a
// project_id, writes .columbus.json, excludes it from git locally, and creates
// the project database. It is idempotent: re-running on an initialized project
// is a no-op that preserves the existing project_id.
func Init(p InitParams) (InitResult, error) {
	configPath := filepath.Join(p.WorkDir, config.FileName)

	if existing, err := config.Load(configPath); err == nil {
		return finishExisting(p, existing.Config, configPath, existing.Warnings)
	} else if !isNotInitialized(err) {
		// A malformed existing config should surface, not be overwritten.
		return InitResult{}, err
	}

	cfg := config.Default()
	cfg.ProjectID = p.IDs.ProjectID()
	if err := config.Save(configPath, cfg); err != nil {
		return InitResult{}, err
	}

	gitExcluded, err := excludeFromGit(p.Ctx, p.WorkDir)
	if err != nil {
		return InitResult{}, err
	}

	paths, err := createStore(p, cfg.ProjectID)
	if err != nil {
		return InitResult{}, err
	}

	return InitResult{
		ProjectID:   cfg.ProjectID,
		ConfigPath:  configPath,
		DataDir:     filepath.Dir(filepath.Dir(paths.ProjectDir)),
		DBPath:      paths.DBPath,
		GitExcluded: gitExcluded,
	}, nil
}

// finishExisting handles the idempotent re-init path: ensure the store and git
// exclude exist, but keep the existing project_id.
func finishExisting(p InitParams, cfg config.Config, configPath string, warnings []string) (InitResult, error) {
	gitExcluded, err := excludeFromGit(p.Ctx, p.WorkDir)
	if err != nil {
		return InitResult{}, err
	}
	paths, err := createStore(p, cfg.ProjectID)
	if err != nil {
		return InitResult{}, err
	}
	return InitResult{
		ProjectID:          cfg.ProjectID,
		ConfigPath:         configPath,
		DataDir:            filepath.Dir(filepath.Dir(paths.ProjectDir)),
		DBPath:             paths.DBPath,
		GitExcluded:        gitExcluded,
		AlreadyInitialized: true,
		Warnings:           warnings,
	}, nil
}

func ctxOrBackground(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

func excludeFromGit(ctx context.Context, workDir string) (bool, error) {
	info, err := gitrepo.DiscoverContext(ctxOrBackground(ctx), workDir)
	if err != nil {
		return false, err
	}
	if err := info.AddExclude(config.FileName); err != nil {
		return false, err
	}
	return info.IsRepo, nil
}

func createStore(p InitParams, projectID string) (config.Paths, error) {
	dataDir, err := config.DataDir(p.Getenv)
	if err != nil {
		return config.Paths{}, err
	}
	paths := config.ProjectPaths(dataDir, projectID)
	if err := os.MkdirAll(paths.ProjectDir, 0o755); err != nil {
		return config.Paths{}, &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
	}
	db, err := store.Open(paths.DBPath)
	if err != nil {
		return config.Paths{}, err
	}
	defer db.Close()
	if err := db.Meta().SetProjectID(projectID); err != nil {
		return config.Paths{}, err
	}
	return paths, nil
}

func isNotInitialized(err error) bool {
	var ce *contract.Error
	return errors.As(err, &ce) && ce.Code == contract.CodeNotInitialized
}
