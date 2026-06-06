package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/rafaelfragoso/columbus/internal/contract"
	"github.com/rafaelfragoso/columbus/internal/render"
)

// errUsage is a small helper for usage errors raised inside the cli layer.
func errUsage(format string, args ...any) *contract.Error {
	return contract.Errorf(contract.CodeUsage, format, args...)
}

// persistentFlags holds the values of the root-level flags shared by all
// commands.
type persistentFlags struct {
	json    bool
	llm     bool
	noColor bool
	version bool
}

// newRootCmd builds the cobra command tree wired to env.
func newRootCmd(env *Env) *cobra.Command {
	pf := &persistentFlags{}

	root := &cobra.Command{
		Use:           "columbus",
		Short:         "Columbus — a local-only, deterministic code-context server",
		SilenceUsage:  true,
		SilenceErrors: true,
		// Root with no subcommand prints version (if --version) or help.
		RunE: func(cmd *cobra.Command, args []string) error {
			if pf.version {
				return renderResult(env, versionResult(env))
			}
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return env.resolveRenderOptions(pf.json, pf.llm, pf.noColor)
		},
	}
	root.SetArgs(nil) // args are supplied via Execute
	root.SetOut(env.Stdout)
	root.SetErr(env.Stderr)

	flags := root.PersistentFlags()
	flags.BoolVar(&pf.json, "json", false, "emit machine-readable JSON on stdout")
	flags.BoolVar(&pf.llm, "llm", false, "emit an LLM-oriented markdown projection")
	flags.BoolVar(&pf.noColor, "no-color", false, "disable ANSI color in text output")
	root.Flags().BoolVar(&pf.version, "version", false, "print version information")

	root.AddCommand(newVersionCmd(env))
	root.AddCommand(newInitCmd(env))
	root.AddCommand(newIndexCmd(env))
	root.AddCommand(newDoctorCmd(env))
	root.AddCommand(newSearchCmd(env))
	root.AddCommand(newShowCmd(env))
	root.AddCommand(newMemoryCmd(env))
	root.AddCommand(newEpicCmd(env))
	root.AddCommand(newTaskCmd(env))
	root.AddCommand(newSelftestCmd(env))

	return root
}

// renderResult writes a successful payload using the env's resolved options.
func renderResult(env *Env, p render.Payload) error {
	return render.Render(env.Stdout, p, env.renderOpts)
}

// Execute runs the CLI with the given args and env, returning the process exit
// code. Domain commands must return *contract.Error for expected failures; any
// other non-nil error is treated as a usage error (the only other source is
// cobra's own flag/arg parsing).
func Execute(args []string, env Env) int {
	root := newRootCmd(&env)
	root.SetArgs(args)

	err := root.Execute()
	if err == nil {
		return env.exitOverride
	}

	ce := toContractError(err)
	command := commandPath(root, args)
	if env.renderOpts.Format == render.FormatJSON {
		_ = render.RenderError(env.Stdout, command, ce, env.renderOpts)
	} else {
		_ = render.RenderError(env.Stderr, command, ce, env.renderOpts)
	}
	return int(ce.Code.Exit())
}

// toContractError coerces an error returned from cobra/Execute into a
// *contract.Error. Domain errors pass through; everything else is a usage
// error (bad flags, unknown command, wrong arg count).
func toContractError(err error) *contract.Error {
	var ce *contract.Error
	if errors.As(err, &ce) {
		return ce
	}
	return contract.Errorf(contract.CodeUsage, "%s", err.Error())
}

// commandPath returns the name of the command that ran, for the error
// envelope. Falls back to "columbus".
func commandPath(root *cobra.Command, args []string) string {
	cmd, _, err := root.Find(args)
	if err != nil || cmd == nil {
		return "columbus"
	}
	if cmd == root {
		return "columbus"
	}
	return cmd.Name()
}
