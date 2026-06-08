package cli

import (
	"github.com/spf13/cobra"

	"github.com/orafaelfragoso/columbus/internal/contract"
	"github.com/orafaelfragoso/columbus/internal/embed"
	"github.com/orafaelfragoso/columbus/internal/extract"
	"github.com/orafaelfragoso/columbus/internal/index"
)

func newIndexCmd(env *Env) *cobra.Command {
	var full, changed, clean, status bool
	var noEmbed bool
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Build or update the project index",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := indexMode(full, changed, clean, status)
			if err != nil {
				return err
			}
			proj, err := env.openProject()
			if err != nil {
				return err
			}
			defer proj.Close()

			reg, err := extract.NewRegistry()
			if err != nil {
				return err
			}

			// Embeddings are best-effort: a missing local runtime degrades to a
			// metadata-only index (search falls back to FTS) rather than failing.
			var embedder index.Embedder
			var embedWarn string
			if !noEmbed && mode != index.ModeStatus && mode != index.ModeClean {
				e, eerr := embed.New(env.ctx())
				if eerr != nil {
					if ce := contract.AsError(eerr); ce.Code == contract.CodeRuntimeMissing {
						embedWarn = "embeddings disabled: " + ce.Message
						proj.Logger.Info("embeddings disabled", "reason", ce.Message)
					} else {
						return eerr
					}
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
				return err
			}
			if embedWarn != "" {
				res.Warnings = append(res.Warnings, embedWarn)
			}
			proj.Logger.Info("index",
				"mode", res.Mode, "indexed", res.Indexed, "unchanged", res.Unchanged,
				"deleted", res.Deleted, "symbols", res.Symbols, "total_files", res.TotalFiles)
			res.Warnings = append(res.Warnings, proj.Warnings...)
			return renderResult(env, res)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&full, "full", false, "reindex everything from scratch (memories preserved)")
	f.BoolVar(&changed, "changed", false, "fast path: only files dirty in the working tree")
	f.BoolVar(&clean, "clean", false, "drop all index data (preserves config and memories)")
	f.BoolVar(&status, "status", false, "report index state without writing")
	f.BoolVar(&noEmbed, "no-embed", false, "skip embeddings (metadata-only index)")
	return cmd
}

func indexMode(full, changed, clean, status bool) (index.Mode, error) {
	count := 0
	for _, b := range []bool{full, changed, clean, status} {
		if b {
			count++
		}
	}
	if count > 1 {
		return 0, errUsage("--full, --changed, --clean and --status are mutually exclusive")
	}
	switch {
	case full:
		return index.ModeFull, nil
	case changed:
		return index.ModeChanged, nil
	case clean:
		return index.ModeClean, nil
	case status:
		return index.ModeStatus, nil
	default:
		return index.ModeIncremental, nil
	}
}
