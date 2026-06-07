// Command columbus is a local-only, deterministic code-context server invoked
// as a tool by a code agent. This is a thin entry point: it wires production
// dependencies, runs the CLI, and exits with the mapped status code.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/orafaelfragoso/columbus/internal/cli"
	"github.com/orafaelfragoso/columbus/internal/clock"
	"github.com/orafaelfragoso/columbus/internal/contract"
	"github.com/orafaelfragoso/columbus/internal/ids"
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
	// A received signal cancels ctx, which propagates to child processes (git,
	// ripgrep) via exec.CommandContext so they are torn down promptly. The
	// signal is recorded so the process can exit with the conventional code.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	caught := make(chan os.Signal, 1)
	go func() {
		s := <-sigCh
		caught <- s
		cancel()
	}()

	env := cli.Env{
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Stdin:   os.Stdin,
		Clock:   clock.System{},
		IDs:     ids.Crypto{},
		WorkDir: wd,
		Getenv:  os.Getenv,
		Ctx:     ctx,
		Version: cli.BuildInfo{Version: version, Commit: commit, Date: date},
	}
	code := cli.Execute(os.Args[1:], env)

	// If a signal interrupted us, prefer the conventional 128+signum code.
	select {
	case s := <-caught:
		if sn, ok := s.(syscall.Signal); ok {
			os.Exit(128 + int(sn))
		}
		os.Exit(130)
	default:
		os.Exit(code)
	}
}
