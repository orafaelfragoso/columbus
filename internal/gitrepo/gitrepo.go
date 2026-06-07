// Package gitrepo wraps the git CLI. git is the single hard runtime dependency.
// Operations are read-only against the repo except for AddExclude, which writes
// only to the local .git/info/exclude (never a tracked file).
package gitrepo

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/orafaelfragoso/columbus/internal/contract"
)

// Info describes the git context of a working directory.
type Info struct {
	// WorkDir is the directory Discover was called against.
	WorkDir string
	// IsRepo is true if WorkDir is inside a git working tree.
	IsRepo bool
	// GitDir is the absolute path to the .git directory (empty if not a repo).
	GitDir string
	// ctx cancels the git subprocesses (e.g. on SIGINT). nil means background.
	ctx context.Context
}

// WithContext returns a copy of Info whose git subprocesses are bound to ctx,
// so cancelling ctx (e.g. on SIGINT/SIGTERM) terminates them promptly.
func (i Info) WithContext(ctx context.Context) Info {
	i.ctx = ctx
	return i
}

func (i Info) context() context.Context {
	if i.ctx != nil {
		return i.ctx
	}
	return context.Background()
}

// Discover inspects workDir using a background context.
func Discover(workDir string) (Info, error) {
	return DiscoverContext(context.Background(), workDir)
}

// DiscoverContext inspects workDir, binding git subprocesses to ctx. Being
// outside a repo is not an error (Columbus supports the non-git path); IsRepo
// is simply false. A cancelled context is reported as an error rather than
// misread as "not a repo".
func DiscoverContext(ctx context.Context, workDir string) (Info, error) {
	out, err := run(ctx, workDir, "rev-parse", "--absolute-git-dir")
	if err != nil {
		if ctx.Err() != nil {
			return Info{}, &contract.Error{Code: contract.CodeStoreError, Message: ctx.Err().Error()}
		}
		// Not a repo: git exits non-zero. Distinguish a missing git binary.
		if _, lookErr := exec.LookPath("git"); lookErr != nil {
			return Info{}, &contract.Error{
				Code:    contract.CodeDependencyMissing,
				Message: "git is required but was not found on PATH",
				Hint:    "install git",
			}
		}
		return Info{WorkDir: workDir, IsRepo: false, ctx: ctx}, nil
	}
	return Info{
		WorkDir: workDir,
		IsRepo:  true,
		GitDir:  strings.TrimSpace(out),
		ctx:     ctx,
	}, nil
}

// AddExclude appends entry to .git/info/exclude if absent (idempotent). It is a
// no-op outside a git repo and never touches a tracked .gitignore.
func (i Info) AddExclude(entry string) error {
	if !i.IsRepo {
		return nil
	}
	infoDir := filepath.Join(i.GitDir, "info")
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		return &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
	}
	excludePath := filepath.Join(infoDir, "exclude")

	existing, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
	}
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == entry {
			return nil
		}
	}

	var buf bytes.Buffer
	buf.Write(existing)
	if len(existing) > 0 && !bytes.HasSuffix(existing, []byte("\n")) {
		buf.WriteByte('\n')
	}
	buf.WriteString(entry + "\n")
	if err := os.WriteFile(excludePath, buf.Bytes(), 0o644); err != nil {
		return &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
	}
	return nil
}

// ListTracked returns repo-relative paths of all tracked files.
func (i Info) ListTracked() ([]string, error) {
	return i.lines("ls-files", "-z")
}

// ListUntracked returns repo-relative paths of untracked files that are not
// excluded by .gitignore (so newly-created, not-yet-committed files index).
func (i Info) ListUntracked() ([]string, error) {
	return i.lines("ls-files", "--others", "--exclude-standard", "-z")
}

// ListModified returns repo-relative paths of tracked files modified in the
// working tree (the fast-path candidate set for --changed).
func (i Info) ListModified() ([]string, error) {
	return i.lines("ls-files", "-m", "-z")
}

// HeadOID returns the current HEAD commit oid, or "" if the repo has no commits.
func (i Info) HeadOID() (string, error) {
	out, err := run(i.context(), i.WorkDir, "rev-parse", "HEAD")
	if err != nil {
		if i.context().Err() != nil {
			return "", &contract.Error{Code: contract.CodeStoreError, Message: i.context().Err().Error()}
		}
		// No commits yet (unborn HEAD) is not an error for our purposes.
		return "", nil
	}
	return strings.TrimSpace(out), nil
}

// Branch returns the current branch name (e.g. "main"). It is best-effort: an
// empty string means not a repo, detached HEAD, or any git error — callers treat
// the branch as cosmetic.
func (i Info) Branch() string {
	if !i.IsRepo {
		return ""
	}
	out, err := run(i.context(), i.WorkDir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	if b := strings.TrimSpace(out); b != "HEAD" {
		return b
	}
	return ""
}

// IsDirty reports whether the working tree has uncommitted changes (modified,
// staged, or untracked-not-ignored). False outside a repo.
func (i Info) IsDirty() (bool, error) {
	if !i.IsRepo {
		return false, nil
	}
	out, err := run(i.context(), i.WorkDir, "status", "--porcelain")
	if err != nil {
		return false, &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
	}
	return strings.TrimSpace(out) != "", nil
}

// lines runs a git command emitting NUL-separated paths and returns them.
func (i Info) lines(args ...string) ([]string, error) {
	out, err := run(i.context(), i.WorkDir, args...)
	if err != nil {
		return nil, &contract.Error{Code: contract.CodeStoreError, Message: "git " + strings.Join(args, " ") + ": " + err.Error()}
	}
	var paths []string
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// run executes a git subcommand in dir and returns stdout. The command is
// bound to ctx, so cancelling ctx terminates the subprocess.
func run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out.String(), nil
}
