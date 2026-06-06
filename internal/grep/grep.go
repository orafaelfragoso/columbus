// Package grep provides live content search over the working tree: a ripgrep
// fast-path when rg is on PATH, and a pure-Go fallback otherwise (so Columbus
// works with git as the only hard dependency). Results are restricted to the
// set of indexed files so each hit can be mapped back to a symbol.
package grep

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Hit is a single content match.
type Hit struct {
	Path string // repo-relative, slash-separated
	Line int    // 1-based
	Text string
}

// Searcher runs a content search.
type Searcher interface {
	// Search finds lines in allowed files containing any token (case-insensitive),
	// returning at most cap hits. allow maps repo-relative paths to inclusion.
	Search(workDir string, tokens []string, allow map[string]bool, cap int) ([]Hit, error)
	// Name identifies the backend (for diagnostics).
	Name() string
}

// New returns the ripgrep searcher when rg is available, else the pure-Go
// fallback.
func New() Searcher {
	if path, err := exec.LookPath("rg"); err == nil {
		return ripgrep{bin: path}
	}
	return goGrep{}
}

// ---- ripgrep fast path ----

type ripgrep struct{ bin string }

func (r ripgrep) Name() string { return "ripgrep" }

func (r ripgrep) Search(workDir string, tokens []string, allow map[string]bool, cap int) ([]Hit, error) {
	if len(tokens) == 0 {
		return nil, nil
	}
	// --fixed-strings: tokens are matched literally, mirroring the pure-Go
	// fallback's substring semantics. Without it ripgrep treats tokens as
	// regex, so a metacharacter token would either match extra lines or fail
	// to compile — diverging from goGrep.
	args := []string{"--json", "-i", "--no-messages", "--fixed-strings"}
	for _, t := range tokens {
		args = append(args, "-e", t)
	}
	args = append(args, "--", ".")
	cmd := exec.Command(r.bin, args...)
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		// rg exits 1 when there are no matches; that is not an error.
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}
	return parseRipgrep(out, allow, cap), nil
}

// rgEvent is the subset of ripgrep --json we consume.
type rgEvent struct {
	Type string `json:"type"`
	Data struct {
		Path       struct{ Text string } `json:"path"`
		Lines      struct{ Text string } `json:"lines"`
		LineNumber int                   `json:"line_number"`
	} `json:"data"`
}

func parseRipgrep(out []byte, allow map[string]bool, cap int) []Hit {
	var hits []Hit
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var ev rgEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil || ev.Type != "match" {
			continue
		}
		rel := strings.TrimPrefix(filepath.ToSlash(ev.Data.Path.Text), "./")
		if !allow[rel] {
			continue
		}
		hits = append(hits, Hit{Path: rel, Line: ev.Data.LineNumber, Text: strings.TrimRight(ev.Data.Lines.Text, "\n")})
		if len(hits) >= cap {
			break
		}
	}
	return hits
}

// ---- pure-Go fallback ----

type goGrep struct{}

func (goGrep) Name() string { return "go" }

func (goGrep) Search(workDir string, tokens []string, allow map[string]bool, cap int) ([]Hit, error) {
	if len(tokens) == 0 {
		return nil, nil
	}
	lowered := make([]string, len(tokens))
	for i, t := range tokens {
		lowered[i] = strings.ToLower(t)
	}

	paths := make([]string, 0, len(allow))
	for p := range allow {
		paths = append(paths, p)
	}
	sort.Strings(paths) // deterministic order

	var hits []Hit
	for _, rel := range paths {
		f, err := os.Open(filepath.Join(workDir, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		lineNo := 0
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for sc.Scan() {
			lineNo++
			line := sc.Text()
			low := strings.ToLower(line)
			if containsAny(low, lowered) {
				hits = append(hits, Hit{Path: rel, Line: lineNo, Text: line})
				if len(hits) >= cap {
					f.Close()
					return hits, nil
				}
			}
		}
		f.Close()
	}
	return hits, nil
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
