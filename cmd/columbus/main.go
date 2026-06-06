// Command columbus is a local-only, deterministic code-context server invoked
// as a tool by a code agent. This is a thin entry point: it wires production
// dependencies, runs the CLI, and exits with the mapped status code.
package main

import (
	"fmt"
	"os"

	"github.com/rafaelfragoso/columbus/internal/cli"
	"github.com/rafaelfragoso/columbus/internal/clock"
	"github.com/rafaelfragoso/columbus/internal/contract"
	"github.com/rafaelfragoso/columbus/internal/ids"
)

// Build metadata, injected via -ldflags at release time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	wd, err := cli.ResolveWorkDir(os.Getwd)
	if err != nil {
		ce := contract.AsError(err)
		fmt.Fprintf(os.Stderr, "error [%s]: %s\n", ce.Code, ce.Message)
		os.Exit(int(ce.Code.Exit()))
	}
	env := cli.Env{
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Stdin:   os.Stdin,
		Clock:   clock.System{},
		IDs:     ids.Crypto{},
		WorkDir: wd,
		Getenv:  os.Getenv,
		Version: cli.BuildInfo{Version: version, Commit: commit, Date: date},
	}
	os.Exit(cli.Execute(os.Args[1:], env))
}
