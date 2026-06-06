package cli

import (
	"context"
	"io"
	"os"

	"github.com/rafaelfragoso/columbus/internal/clock"
	"github.com/rafaelfragoso/columbus/internal/contract"
	"github.com/rafaelfragoso/columbus/internal/ids"
	"github.com/rafaelfragoso/columbus/internal/render"
)

// ResolveWorkDir returns the working directory from getwd (os.Getwd in
// production), mapping a failure to a clear error instead of silently using ""
// — an empty WorkDir otherwise produces confusing path errors deep in later
// operations.
func ResolveWorkDir(getwd func() (string, error)) (string, error) {
	wd, err := getwd()
	if err != nil {
		return "", contract.Errorf(contract.CodeStoreError,
			"cannot determine working directory: %v", err)
	}
	return wd, nil
}

// BuildInfo holds version metadata injected at build time via -ldflags.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Env carries all injected dependencies and I/O streams for a single CLI
// invocation. It keeps cobra commands testable in-process: tests construct an
// Env with buffers and deterministic clock/ids and call Execute directly.
type Env struct {
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader

	Clock clock.Clock
	IDs   ids.Source

	// WorkDir is the project working directory (defaults to os cwd).
	WorkDir string
	// DataDir overrides the resolved data directory when non-empty (tests set
	// this; production resolves from COLUMBUS_DATA_DIR / OS conventions).
	DataDir string

	// Getenv reads environment variables (injected for test isolation).
	Getenv func(string) string

	// Ctx cancels long-running work and its child processes (git, ripgrep) on
	// SIGINT/SIGTERM. nil is treated as context.Background().
	Ctx context.Context

	Version BuildInfo

	// renderOpts is resolved from persistent flags in PersistentPreRunE.
	renderOpts render.Options
	// exitOverride lets a command that renders its own payload still request a
	// non-zero exit (e.g. doctor reporting failed checks) without emitting an
	// error envelope. 0 means "use the normal mapping".
	exitOverride int
}

// setExit requests a process exit code for a command that has already rendered
// its payload.
func (e *Env) setExit(code int) { e.exitOverride = code }

// ctx returns the cancellation context, defaulting to background.
func (e *Env) ctx() context.Context {
	if e.Ctx != nil {
		return e.Ctx
	}
	return context.Background()
}

// resolveRenderOptions computes the render options from the parsed flags.
func (e *Env) resolveRenderOptions(asJSON, asLLM, noColor bool) error {
	format := render.FormatText
	switch {
	case asJSON && asLLM:
		return errUsage("--json and --llm are mutually exclusive")
	case asJSON:
		format = render.FormatJSON
	case asLLM:
		format = render.FormatLLM
	}

	e.renderOpts = render.Options{Format: format, Color: e.wantColor(format, noColor)}
	return nil
}

// wantColor resolves ANSI color in priority order (first match wins): structured
// formats never color; --no-color and NO_COLOR force off; FORCE_COLOR forces on;
// TERM=dumb and CI force off; otherwise color follows whether stdout is a TTY.
func (e *Env) wantColor(format render.Format, noColor bool) bool {
	if format != render.FormatText {
		return false
	}
	switch {
	case noColor, e.Getenv("NO_COLOR") != "":
		return false
	case e.Getenv("FORCE_COLOR") != "":
		return true
	case e.Getenv("TERM") == "dumb", e.Getenv("CI") != "":
		return false
	default:
		return isTTY(e.Stdout)
	}
}

// isTTY reports whether w is a character device (terminal).
func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
