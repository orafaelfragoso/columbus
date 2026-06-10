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
		// RunE makes unknown subcommands a usage error (exit 2) instead of
		// silently printing help with exit 0.
		RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newShowSymbolCmd(env), newShowFileCmd(env), newShowMemoryCmd(env))
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
