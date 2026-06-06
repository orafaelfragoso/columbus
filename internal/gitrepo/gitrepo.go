// Package gitrepo wraps the git CLI. git is the single hard runtime dependency.
// Operations are read-only against the repo except for AddExclude, which writes
// only to the local .git/info/exclude (never a tracked file).
package gitrepo

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rafaelfragoso/columbus/internal/contract"
)

// Info describes the git context of a working directory.
type Info struct {
	// WorkDir is the directory Discover was called against.
	WorkDir string
	// IsRepo is true if WorkDir is inside a git working tree.
	IsRepo bool
	// GitDir is the absolute path to the .git directory (empty if not a repo).
	GitDir string
}

// Discover inspects workDir. Being outside a repo is not an error (Columbus
// supports the non-git path); IsRepo is simply false.
func Discover(workDir string) (Info, error) {
	out, err := run(workDir, "rev-parse", "--absolute-git-dir")
	if err != nil {
		// Not a repo: git exits non-zero. Distinguish a missing git binary.
		if _, lookErr := exec.LookPath("git"); lookErr != nil {
			return Info{}, &contract.Error{
				Code:    contract.CodeDependencyMissing,
				Message: "git is required but was not found on PATH",
				Hint:    "install git",
			}
		}
		return Info{WorkDir: workDir, IsRepo: false}, nil
	}
	return Info{
		WorkDir: workDir,
		IsRepo:  true,
		GitDir:  strings.TrimSpace(out),
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
	out, err := run(i.WorkDir, "rev-parse", "HEAD")
	if err != nil {
		// No commits yet (unborn HEAD) is not an error for our purposes.
		return "", nil
	}
	return strings.TrimSpace(out), nil
}

// IsDirty reports whether the working tree has uncommitted changes (modified,
// staged, or untracked-not-ignored). False outside a repo.
func (i Info) IsDirty() (bool, error) {
	if !i.IsRepo {
		return false, nil
	}
	out, err := run(i.WorkDir, "status", "--porcelain")
	if err != nil {
		return false, &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
	}
	return strings.TrimSpace(out) != "", nil
}

// lines runs a git command emitting NUL-separated paths and returns them.
func (i Info) lines(args ...string) ([]string, error) {
	out, err := run(i.WorkDir, args...)
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

// run executes a git subcommand in dir and returns stdout.
func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out.String(), nil
}
