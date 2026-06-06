package cli

import (
	"github.com/spf13/cobra"

	"github.com/rafaelfragoso/columbus/internal/extract"
	"github.com/rafaelfragoso/columbus/internal/render"
	"github.com/rafaelfragoso/columbus/internal/show"
)

func newShowCmd(env *Env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show a symbol, file, or memory in detail",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newShowSymbolCmd(env), newShowFileCmd(env), newShowMemoryCmd(env))
	return cmd
}

// withShower opens the project, builds a Shower and a registry, and invokes fn.
func withShower(env *Env, fn func(*show.Shower) (render.Payload, error)) error {
	proj, err := env.openProject()
	if err != nil {
		return err
	}
	defer proj.DB.Close()
	reg, err := extract.NewRegistry()
	if err != nil {
		return err
	}
	payload, err := fn(&show.Shower{DB: proj.DB, WorkDir: env.WorkDir, Registry: reg})
	if err != nil {
		return err
	}
	return renderResult(env, payload)
}

func newShowSymbolCmd(env *Env) *cobra.Command {
	var in string
	var contextLines int
	cmd := &cobra.Command{
		Use:   "symbol <name>",
		Short: "Show all definitions of a symbol",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withShower(env, func(s *show.Shower) (render.Payload, error) {
				return s.Symbol(args[0], in, contextLines)
			})
		},
	}
	cmd.Flags().StringVar(&in, "in", "", "narrow to files whose path contains this substring")
	cmd.Flags().IntVar(&contextLines, "context-lines", 3, "lines of context around the definition")
	return cmd
}

func newShowFileCmd(env *Env) *cobra.Command {
	var contextLines int
	cmd := &cobra.Command{
		Use:   "file <path>",
		Short: "Show a file's outline and graph",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withShower(env, func(s *show.Shower) (render.Payload, error) {
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
			return withShower(env, func(s *show.Shower) (render.Payload, error) {
				return s.Memory(args[0])
			})
		},
	}
}
