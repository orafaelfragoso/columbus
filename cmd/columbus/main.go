// Command columbus is a local-only, deterministic code-context server invoked
// as a tool by a code agent. This is a thin entry point: it wires production
// dependencies, runs the CLI, and exits with the mapped status code.
package main

import (
	"os"

	"github.com/rafaelfragoso/columbus/internal/cli"
	"github.com/rafaelfragoso/columbus/internal/clock"
	"github.com/rafaelfragoso/columbus/internal/ids"
)

// Build metadata, injected via -ldflags at release time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	wd, _ := os.Getwd()
	env := cli.Env{
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Clock:   clock.System{},
		IDs:     ids.Crypto{},
		WorkDir: wd,
		Getenv:  os.Getenv,
		Version: cli.BuildInfo{Version: version, Commit: commit, Date: date},
	}
	os.Exit(cli.Execute(os.Args[1:], env))
}
