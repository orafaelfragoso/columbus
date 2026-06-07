package index

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/orafaelfragoso/columbus/internal/contract"
	"github.com/orafaelfragoso/columbus/internal/gitrepo"
)

// candidate is a file selected for (potential) indexing.
type candidate struct {
	// RelPath is the repo-relative, slash-separated path.
	RelPath string
	// Tracked reports whether git tracks the file (chooses blob_oid vs sha256).
	Tracked bool
}

// selectFiles computes the candidate file set. In a git repo it is
// tracked ∪ untracked-not-ignored (so .gitignore is honored for free); outside
// git it is a filesystem walk honoring the Columbus exclude globs. onlyDirty
// restricts a git repo to modified+untracked files (the --changed fast path).
func selectFiles(workDir string, git gitrepo.Info, excludes []string, onlyDirty bool) ([]candidate, error) {
	if git.IsRepo {
		return selectGit(git, onlyDirty)
	}
	return selectWalk(workDir, excludes)
}

func selectGit(git gitrepo.Info, onlyDirty bool) ([]candidate, error) {
	set := map[string]bool{} // relpath -> tracked
	tracked := map[string]bool{}

	if onlyDirty {
		modified, err := git.ListModified()
		if err != nil {
			return nil, err
		}
		for _, p := range modified {
			set[p] = true
			tracked[p] = true
		}
	} else {
		all, err := git.ListTracked()
		if err != nil {
			return nil, err
		}
		for _, p := range all {
			set[p] = true
			tracked[p] = true
		}
	}

	untracked, err := git.ListUntracked()
	if err != nil {
		return nil, err
	}
	for _, p := range untracked {
		set[p] = true
	}

	return toSortedCandidates(set, tracked), nil
}

func selectWalk(workDir string, excludes []string) ([]candidate, error) {
	set := map[string]bool{}
	err := filepath.WalkDir(workDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(workDir, path)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if rel == ".git" || matchesAny(rel+"/", excludes) || matchesAny(rel, excludes) {
				return fs.SkipDir
			}
			return nil
		}
		if matchesAny(rel, excludes) {
			return nil
		}
		set[rel] = false
		return nil
	})
	if err != nil {
		return nil, &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
	}
	return toSortedCandidates(set, map[string]bool{}), nil
}

func toSortedCandidates(set, tracked map[string]bool) []candidate {
	out := make([]candidate, 0, len(set))
	for p := range set {
		out = append(out, candidate{RelPath: p, Tracked: tracked[p]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out
}

// matchesAny reports whether rel matches any exclude glob (doublestar). Patterns
// are matched against the full relative path; a bare pattern like "node_modules"
// also matches as a path segment via the **/ prefix.
func matchesAny(rel string, excludes []string) bool {
	for _, pat := range excludes {
		if ok, _ := doublestar.Match(pat, rel); ok {
			return true
		}
		if !strings.Contains(pat, "/") {
			if ok, _ := doublestar.Match("**/"+pat+"/**", rel); ok {
				return true
			}
		}
	}
	return false
}

// readFile reads a candidate's bytes from disk.
func readFile(workDir, rel string) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(workDir, filepath.FromSlash(rel)))
	if err != nil {
		return nil, err
	}
	return b, nil
}
