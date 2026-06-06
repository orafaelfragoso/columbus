package index

import (
	"path"
	"strings"
)

// FileMeta is the subset of a file row used for graph resolution.
type FileMeta struct {
	ID       int64
	Path     string
	Role     string
	Language string
}

// ImportRow is a raw import specifier tied to its file.
type ImportRow struct {
	FileID    int64
	Specifier string
}

// DepEdge is a resolved file -> file dependency.
type DepEdge struct {
	From int64
	To   int64
}

// TestLink associates an implementation file with its test file.
type TestLink struct {
	Impl int64
	Test int64
}

// resolveExtensions are tried (in order) when resolving an extension-less
// relative import to an indexed file.
var resolveExtensions = []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".py"}

// resolveGraph computes best-effort dependency edges (relative/same-package
// imports only) and test links (by filename convention). Unresolvable imports
// produce no edge.
func resolveGraph(files []FileMeta, imports []ImportRow) ([]DepEdge, []TestLink) {
	byPath := make(map[string]int64, len(files))
	for _, f := range files {
		byPath[f.Path] = f.ID
	}

	var edges []DepEdge
	seenEdge := map[[2]int64]bool{}
	pathOf := make(map[int64]string, len(files))
	for _, f := range files {
		pathOf[f.ID] = f.Path
	}
	for _, imp := range imports {
		if !strings.HasPrefix(imp.Specifier, ".") {
			continue // bare/package import: best-effort skips it
		}
		toID, ok := resolveRelative(pathOf[imp.FileID], imp.Specifier, byPath)
		if !ok || toID == imp.FileID {
			continue
		}
		key := [2]int64{imp.FileID, toID}
		if seenEdge[key] {
			continue
		}
		seenEdge[key] = true
		edges = append(edges, DepEdge{From: imp.FileID, To: toID})
	}

	var links []TestLink
	for _, f := range files {
		if f.Role != "test" {
			continue
		}
		for _, cand := range implCandidates(f.Path) {
			if implID, ok := byPath[cand]; ok && implID != f.ID {
				links = append(links, TestLink{Impl: implID, Test: f.ID})
				break
			}
		}
	}
	return edges, links
}

// resolveRelative resolves a relative import specifier from importerPath to an
// indexed file id.
func resolveRelative(importerPath, specifier string, byPath map[string]int64) (int64, bool) {
	base := path.Dir(importerPath)
	target := path.Clean(path.Join(base, specifier))

	candidates := []string{target}
	for _, ext := range resolveExtensions {
		candidates = append(candidates, target+ext)
	}
	for _, ext := range resolveExtensions {
		candidates = append(candidates, path.Join(target, "index"+ext))
	}
	for _, c := range candidates {
		if id, ok := byPath[c]; ok {
			return id, true
		}
	}
	return 0, false
}

// implCandidates returns possible implementation paths for a test file, based
// on naming conventions.
func implCandidates(testPath string) []string {
	dir := path.Dir(testPath)
	base := path.Base(testPath)
	var stems []string

	switch {
	case strings.HasSuffix(base, "_test.go"):
		stems = append(stems, strings.TrimSuffix(base, "_test.go")+".go")
	case strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py"):
		stems = append(stems, strings.TrimPrefix(base, "test_"))
	case strings.HasSuffix(base, "_test.py"):
		stems = append(stems, strings.TrimSuffix(base, "_test.py")+".py")
	default:
		// foo.test.ext / foo.spec.ext -> foo.<known ext>
		for _, marker := range []string{".test.", ".spec."} {
			if i := strings.Index(base, marker); i >= 0 {
				stem := base[:i]
				for _, ext := range []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"} {
					stems = append(stems, stem+ext)
				}
			}
		}
	}

	out := make([]string, 0, len(stems))
	for _, s := range stems {
		out = append(out, path.Join(dir, s))
	}
	return out
}
