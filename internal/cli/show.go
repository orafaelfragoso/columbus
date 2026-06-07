package cli

import (
	"github.com/spf13/cobra"

	"github.com/orafaelfragoso/columbus/internal/extract"
	"github.com/orafaelfragoso/columbus/internal/render"
	"github.com/orafaelfragoso/columbus/internal/show"
)

func newShowCmd(env *Env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show a symbol, file, or memory in detail",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newShowSymbolCmd(env), newShowFileCmd(env), newShowMemoryCmd(env),
		newShowEpicCmd(env), newShowTaskCmd(env), newShowGraphCmd(env))
	return cmd
}

func newShowGraphCmd(env *Env) *cobra.Command {
	var in, role, lang string
	var max int
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Project the indexed file dependency graph (nodes + edges)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withShower(env, "show.graph", func(s *show.Shower) (render.Payload, error) {
				return s.Graph(show.GraphOptions{In: in, Role: role, Lang: lang, Max: max})
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&in, "in", "", "keep files whose path contains this substring")
	f.StringVar(&role, "role", "", "keep files with this role (impl|test|...)")
	f.StringVar(&lang, "lang", "", "keep files with this language")
	f.IntVar(&max, "max", 0, "cap the number of file nodes (0 = default 200)")
	return cmd
}

// withShower opens the project, builds a Shower and a registry, and invokes fn.
// show is a read, so it logs at debug.
func withShower(env *Env, cmdName string, fn func(*show.Shower) (render.Payload, error)) error {
	proj, err := env.openProject()
	if err != nil {
		return err
	}
	defer proj.Close()
	reg, err := extract.NewRegistry()
	if err != nil {
		return err
	}
	payload, err := fn(&show.Shower{DB: proj.DB, WorkDir: env.WorkDir, Registry: reg, Logger: proj.Logger})
	if err != nil {
		proj.Logger.Info(cmdName+" failed", "error", err.Error())
		return err
	}
	proj.Logger.Debug(cmdName)
	return renderResult(env, payload)
}

func newShowSymbolCmd(env *Env) *cobra.Command {
	var in string
	var contextLines int
	var snippetLines int
	cmd := &cobra.Command{
		Use:   "symbol <name>",
		Short: "Show all definitions of a symbol",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withShower(env, "show.symbol", func(s *show.Shower) (render.Payload, error) {
				return s.Symbol(args[0], in, contextLines, snippetLines)
			})
		},
	}
	cmd.Flags().StringVar(&in, "in", "", "narrow to files whose path contains this substring")
	cmd.Flags().IntVar(&contextLines, "context-lines", 3, "lines of context around the definition")
	cmd.Flags().IntVar(&snippetLines, "snippet-lines", 0, "cap snippet length in lines (0 = default 60)")
	return cmd
}

func newShowFileCmd(env *Env) *cobra.Command {
	var contextLines int
	cmd := &cobra.Command{
		Use:   "file <path>",
		Short: "Show a file's outline and graph",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withShower(env, "show.file", func(s *show.Shower) (render.Payload, error) {
				return s.File(args[0], contextLines)
			})
		},
	}
	cmd.Flags().IntVar(&contextLines, "context-lines", 3, "lines of context (reserved)")
	return cmd
}

func newShowMemoryCmd(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "memory <id>",
		Short: "Show a memory by id (mem_NNN)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withShower(env, "show.memory", func(s *show.Shower) (render.Payload, error) {
				return s.Memory(args[0])
			})
		},
	}
}

func newShowEpicCmd(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "epic <id>",
		Short: "Show an epic by id (epic_NNN): fields, refs, history, child tasks",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withShower(env, "show.epic", func(s *show.Shower) (render.Payload, error) {
				return s.Epic(args[0])
			})
		},
	}
}

func newShowTaskCmd(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "task <id>",
		Short: "Show a task by id (task_NNN): fields, refs, history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withShower(env, "show.task", func(s *show.Shower) (render.Payload, error) {
				return s.Task(args[0])
			})
		},
	}
}
