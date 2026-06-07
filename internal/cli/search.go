package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/orafaelfragoso/columbus/internal/extract"
	"github.com/orafaelfragoso/columbus/internal/grep"
	"github.com/orafaelfragoso/columbus/internal/search"
)

func newSearchCmd(env *Env) *cobra.Command {
	var (
		kindFlag     string
		limit        int
		contextLines int
		graph        bool
		snippets     bool
		snippetLines int
	)
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search the codebase and memories for LLM-ready context",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			kind, err := search.ParseKind(kindFlag)
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
			engine := &search.Engine{
				DB:       proj.DB,
				WorkDir:  env.WorkDir,
				Registry: reg,
				Searcher: grep.NewContext(env.ctx()),
				Logger:   proj.Logger,
			}
			res, err := engine.Search(search.Query{
				Text:         strings.Join(args, " "),
				Kind:         kind,
				Limit:        limit,
				ContextLines: contextLines,
				Graph:        graph,
				Snippets:     snippets,
				SnippetLines: snippetLines,
			})
			if err != nil {
				proj.Logger.Info("search failed", "error", err.Error())
				return err
			}
			proj.Logger.Debug("search", "kind", kind.String(), "results", res.Total)
			res.Warnings = append(res.Warnings, proj.Warnings...)
			return renderResult(env, res)
		},
	}
	f := cmd.Flags()
	f.StringVar(&kindFlag, "kind", "all", "what to search: code|memory|epic|task|all")
	f.IntVar(&limit, "limit", 20, "maximum number of results")
	f.IntVar(&contextLines, "context-lines", 3, "lines of context around matched ranges")
	f.BoolVar(&graph, "graph", false, "include 1-hop graph neighbors (imports/imported-by)")
	f.BoolVar(&snippets, "snippets", false, "attach code bodies (default: locations, signatures, scores and graph edges only)")
	f.IntVar(&snippetLines, "snippet-lines", 0, "cap snippet length in lines when --snippets is set (0 = default 60)")
	return cmd
}
