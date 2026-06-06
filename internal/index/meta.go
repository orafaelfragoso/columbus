package index

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// gitBlobOID computes the git blob object id of content (sha1 of the git blob
// header + content), matching `git hash-object`.
func gitBlobOID(content []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(content))
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}

// sha256hex returns the hex sha256 of content.
func sha256hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// isBinary uses the common NUL-byte heuristic over a prefix of the content.
func isBinary(content []byte) bool {
	const sniff = 8000
	if len(content) > sniff {
		content = content[:sniff]
	}
	return bytes.IndexByte(content, 0) >= 0
}

// deriveRole classifies a file by path heuristics.
func deriveRole(path string) string {
	base := strings.ToLower(filepath.Base(path))
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(base, "_test.go"),
		strings.Contains(base, ".test."),
		strings.Contains(base, ".spec."),
		strings.Contains(lower, "/__tests__/"):
		return "test"
	case strings.HasSuffix(base, ".md"), strings.HasSuffix(base, ".markdown"):
		return "doc"
	default:
		return "impl"
	}
}

// manifestNames are the package-defining manifests, nearest wins.
var manifestNames = []string{"go.mod", "package.json", "pyproject.toml"}

// packageResolver derives a file's package as the directory name of the nearest
// enclosing manifest, caching per directory.
type packageResolver struct {
	workDir string
	mu      sync.Mutex
	cache   map[string]string
}

func newPackageResolver(workDir string) *packageResolver {
	return &packageResolver{workDir: workDir, cache: map[string]string{}}
}

// pkgFor returns the package for a repo-relative file path. Safe for concurrent
// use by parse workers.
func (r *packageResolver) pkgFor(relPath string) string {
	dir := filepath.Dir(filepath.Join(r.workDir, relPath))
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.forDir(dir)
}

// forDir must be called with r.mu held.
func (r *packageResolver) forDir(dir string) string {
	if v, ok := r.cache[dir]; ok {
		return v
	}
	pkg := ""
	for _, m := range manifestNames {
		if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
			pkg = filepath.Base(dir)
			break
		}
	}
	if pkg == "" {
		parent := filepath.Dir(dir)
		// Stop at the work dir root or filesystem root.
		if parent != dir && len(parent) >= len(r.workDir) && strings.HasPrefix(dir, r.workDir) {
			pkg = r.forDir(parent)
		}
	}
	r.cache[dir] = pkg
	return pkg
}
