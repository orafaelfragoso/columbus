package cli

import (
	"io"
	"os"

	"github.com/rafaelfragoso/columbus/internal/clock"
	"github.com/rafaelfragoso/columbus/internal/ids"
	"github.com/rafaelfragoso/columbus/internal/render"
)

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

	Clock clock.Clock
	IDs   ids.Source

	// WorkDir is the project working directory (defaults to os cwd).
	WorkDir string
	// DataDir overrides the resolved data directory when non-empty (tests set
	// this; production resolves from COLUMBUS_DATA_DIR / OS conventions).
	DataDir string

	// Getenv reads environment variables (injected for test isolation).
	Getenv func(string) string

	Version BuildInfo

	// renderOpts is resolved from persistent flags in PersistentPreRunE.
	renderOpts render.Options
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

	color := false
	if format == render.FormatText && !noColor && e.Getenv("NO_COLOR") == "" {
		color = isTTY(e.Stdout)
	}
	e.renderOpts = render.Options{Format: format, Color: color}
	return nil
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
