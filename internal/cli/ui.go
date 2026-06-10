package cli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/orafaelfragoso/columbus/internal/contract"
	"github.com/orafaelfragoso/columbus/internal/embed"
	"github.com/orafaelfragoso/columbus/internal/extract"
	"github.com/orafaelfragoso/columbus/internal/gitrepo"
	"github.com/orafaelfragoso/columbus/internal/grep"
	"github.com/orafaelfragoso/columbus/internal/index"
	"github.com/orafaelfragoso/columbus/internal/memory"
	"github.com/orafaelfragoso/columbus/internal/search"
	"github.com/orafaelfragoso/columbus/internal/tui"
)

// newViewCmd builds `columbus view`: a full-screen, read-mostly dashboard over
// the project's index, memory and embeddings.
func newViewCmd(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "view",
		Short: "open the interactive dashboard (index, memory, embeddings)",
		Long: "Open a full-screen terminal dashboard over the indexed project: index " +
			"freshness, embeddings and durable memory. Read-mostly — arrows to " +
			"navigate, enter for detail, / to semantic-search code/memory, " +
			"r to refresh, R to reindex, q to quit.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			proj, err := env.openProject()
			if err != nil {
				return err
			}
			defer proj.Close()

			reg, err := extract.NewRegistry()
			if err != nil {
				return err
			}

			mgr := &memory.Manager{DB: proj.DB, Clock: env.Clock, WorkDir: env.WorkDir}
			src := &tui.StoreSource{DB: proj.DB, Memory: mgr, Branch: currentBranch(env)}

			// Headless one-frame render for debugging / non-tty environments.
			if env.Getenv("COLUMBUS_UI_PRINT") != "" {
				w := envInt(env, "COLUMBUS_UI_W", 168)
				h := envInt(env, "COLUMBUS_UI_H", 44)
				return tui.PrintFrame(env.Stdout, src, w, h)
			}

			var embedder search.Embedder
			if e, eerr := embed.New(env.ctx()); eerr != nil {
				if ce := contract.AsError(eerr); ce.Code == contract.CodeRuntimeMissing {
					proj.Logger.Info("semantic search disabled", "reason", ce.Message)
				} else {
					return eerr
				}
			} else {
				defer e.Close()
				embedder = e
			}

			return tui.Run(src,
				tui.WithRefreshInterval(2*time.Second),
				tui.WithReindex(reindexFunc(env, proj, reg)),
				tui.WithSearch(searchFunc(env, proj, reg, embedder)),
			)
		},
	}
}

// reindexFunc runs an in-process incremental index over the open project.
func reindexFunc(env *Env, proj *projectContext, reg *extract.Registry) func() error {
	return func() error {
		ix := &index.Indexer{
			DB:          proj.DB,
			Registry:    reg,
			WorkDir:     env.WorkDir,
			Clock:       env.Clock,
			MaxFileSize: proj.Config.Indexing.MaxFileSize,
			Excludes:    proj.Config.Indexing.Exclude,
			Ctx:         env.ctx(),
		}
		_, err := ix.Run(index.ModeIncremental)
		return err
	}
}

// searchFunc runs a global ranked semantic search across code and memory (the
// same engine `columbus search --kind all` uses) and maps it to SearchHits.
func searchFunc(env *Env, proj *projectContext, reg *extract.Registry, embedder search.Embedder) func(string) ([]tui.SearchHit, error) {
	return func(q string) ([]tui.SearchHit, error) {
		engine := &search.Engine{
			DB:       proj.DB,
			WorkDir:  env.WorkDir,
			Registry: reg,
			Searcher: grep.NewContext(env.ctx()),
			Embedder: embedder,
			Logger:   proj.Logger,
		}
		res, err := engine.Search(search.Query{Text: q, Kind: search.KindAll, Limit: 50, ContextLines: 3, Snippets: true})
		if err != nil {
			return nil, err
		}
		hits := make([]tui.SearchHit, 0, len(res.Hits)+len(res.Memories))
		for _, h := range res.Hits {
			hits = append(hits, tui.SearchHit{
				Grain: h.Grain, Title: h.Name, Where: hitWhere(h),
				Score: h.Score, Snippet: h.Snippet,
			})
		}
		for _, m := range res.Memories {
			hits = append(hits, tui.SearchHit{
				Grain: "memory", ID: m.ID, Title: m.Title, Where: m.ID,
				Score: m.Score, Snippet: m.Body,
			})
		}
		return hits, nil
	}
}

func hitWhere(h search.Hit) string {
	if h.StartLine > 0 {
		return fmt.Sprintf("%s:%d", h.Path, h.StartLine)
	}
	return h.Path
}

// currentBranch resolves the working-tree branch for the header. Best-effort:
// any failure yields "" and the header simply omits the branch.
func currentBranch(env *Env) string {
	info, err := gitrepo.DiscoverContext(env.ctx(), env.WorkDir)
	if err != nil {
		return ""
	}
	return info.Branch()
}

func envInt(env *Env, key string, def int) int {
	if v, err := strconv.Atoi(env.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}
