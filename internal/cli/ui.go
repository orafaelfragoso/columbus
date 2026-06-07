package cli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/rafaelfragoso/columbus/internal/extract"
	"github.com/rafaelfragoso/columbus/internal/gitrepo"
	"github.com/rafaelfragoso/columbus/internal/grep"
	"github.com/rafaelfragoso/columbus/internal/index"
	"github.com/rafaelfragoso/columbus/internal/memory"
	"github.com/rafaelfragoso/columbus/internal/search"
	"github.com/rafaelfragoso/columbus/internal/tui"
)

// newUICmd builds `columbus ui`: a full-screen, read-mostly dashboard over the
// project's index, memory, and structured work (epics & tasks).
func newUICmd(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "ui",
		Short: "open the interactive dashboard (index, memory, epics, tasks, graph)",
		Long: "Open a full-screen terminal dashboard over the indexed project: index " +
			"freshness, durable memory, epics & tasks, and the dependency-graph hubs. " +
			"Read-mostly — tab/arrows to navigate, enter for detail, / to search " +
			"code/memory/work, r to refresh, R to reindex, q to quit.",
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

			return tui.Run(src,
				tui.WithRefreshInterval(2*time.Second),
				tui.WithReindex(reindexFunc(env, proj, reg)),
				tui.WithSearch(searchFunc(env, proj, reg)),
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

// searchFunc runs a global ranked search across code, memory, epics and tasks
// (the same engine `columbus search --kind all` uses) and maps it to SearchHits.
func searchFunc(env *Env, proj *projectContext, reg *extract.Registry) func(string) ([]tui.SearchHit, error) {
	return func(q string) ([]tui.SearchHit, error) {
		engine := &search.Engine{
			DB:       proj.DB,
			WorkDir:  env.WorkDir,
			Registry: reg,
			Searcher: grep.NewContext(env.ctx()),
			Logger:   proj.Logger,
		}
		res, err := engine.Search(search.Query{Text: q, Kind: search.KindAll, Limit: 50})
		if err != nil {
			return nil, err
		}
		hits := make([]tui.SearchHit, 0, len(res.Hits))
		for _, h := range res.Hits {
			hits = append(hits, tui.SearchHit{
				Grain: h.Grain, ID: h.ID, Title: hitTitle(h), Where: hitWhere(h),
			})
		}
		return hits, nil
	}
}

func hitTitle(h search.Hit) string {
	if h.Name != "" {
		return h.Name
	}
	return h.ID
}

func hitWhere(h search.Hit) string {
	if h.Path != "" {
		if h.StartLine > 0 {
			return fmt.Sprintf("%s:%d", h.Path, h.StartLine)
		}
		return h.Path
	}
	return h.ID
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
