package cli

import (
	"github.com/orafaelfragoso/columbus/internal/contract"
	"github.com/orafaelfragoso/columbus/internal/embed"
	"github.com/orafaelfragoso/columbus/internal/extract"
	"github.com/orafaelfragoso/columbus/internal/index"
)

// runIndex opens the project, builds an embedder (best-effort) and runs the
// indexer in the given mode. Embeddings degrade gracefully: a missing local
// runtime yields a metadata-only index and a warning rather than an error.
// Reindex and install both flow through here.
func runIndex(env *Env, mode index.Mode, noEmbed bool) (index.IndexResult, []string, error) {
	proj, err := env.openProject()
	if err != nil {
		return index.IndexResult{}, nil, err
	}
	defer proj.Close()

	reg, err := extract.NewRegistry()
	if err != nil {
		return index.IndexResult{}, nil, err
	}

	var embedder index.Embedder
	var warnings []string
	wantEmbed := !noEmbed && proj.Config.Embedding.Enabled &&
		mode != index.ModeStatus && mode != index.ModeClean
	if wantEmbed {
		e, eerr := embed.New(env.ctx())
		if eerr != nil {
			ce := contract.AsError(eerr)
			if ce.Code != contract.CodeRuntimeMissing {
				return index.IndexResult{}, nil, eerr
			}
			warnings = append(warnings, "embeddings disabled: "+ce.Message)
			proj.Logger.Info("embeddings disabled", "reason", ce.Message)
		} else {
			defer e.Close()
			embedder = e
		}
	}

	ix := &index.Indexer{
		DB:          proj.DB,
		Registry:    reg,
		WorkDir:     env.WorkDir,
		Clock:       env.Clock,
		MaxFileSize: proj.Config.Indexing.MaxFileSize,
		Excludes:    proj.Config.Indexing.Exclude,
		Embedder:    embedder,
		Ctx:         env.ctx(),
	}
	res, err := ix.Run(mode)
	if err != nil {
		proj.Logger.Info("index failed", "mode", mode.String(), "error", err.Error())
		return index.IndexResult{}, nil, err
	}
	proj.Logger.Info("index",
		"mode", res.Mode, "indexed", res.Indexed, "unchanged", res.Unchanged,
		"deleted", res.Deleted, "symbols", res.Symbols, "total_files", res.TotalFiles)
	warnings = append(warnings, proj.Warnings...)
	return res, warnings, nil
}
