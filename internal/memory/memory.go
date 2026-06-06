// Package memory owns the durable project memory: records, evidence, links,
// tags, validation and drift detection. Memories are timeless; only their
// evidence and links carry git anchors used to detect drift (always a warning,
// never a hard invalid).
package memory

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rafaelfragoso/columbus/internal/clock"
	"github.com/rafaelfragoso/columbus/internal/contract"
	"github.com/rafaelfragoso/columbus/internal/store"
)

// Kinds is the fixed memory-kind enum.
var Kinds = []string{"decision", "pattern", "failure", "command", "glossary"}

func validKind(k string) bool {
	for _, v := range Kinds {
		if v == k {
			return true
		}
	}
	return false
}

// FormatID renders a numeric id as mem_NNN.
func FormatID(id int64) string { return fmt.Sprintf("mem_%03d", id) }

// ParseID parses a mem_NNN identifier.
func ParseID(id string) (int64, error) {
	v, err := strconv.ParseInt(strings.TrimPrefix(id, "mem_"), 10, 64)
	if err != nil || v <= 0 {
		return 0, contract.Errorf(contract.CodeUsage, "invalid memory id %q (want mem_NNN)", id)
	}
	return v, nil
}

// EvidenceSpec is a parsed --evidence value.
type EvidenceSpec struct {
	Path  string
	Start int
	End   int
}

// ParseEvidence parses "path:start-end" or "path:start".
func ParseEvidence(s string) (EvidenceSpec, error) {
	i := strings.LastIndexByte(s, ':')
	if i <= 0 || i == len(s)-1 {
		return EvidenceSpec{}, contract.Errorf(contract.CodeUsage, "invalid evidence %q (want path:start-end)", s)
	}
	path, rng := s[:i], s[i+1:]
	start, end, err := parseRange(rng)
	if err != nil {
		return EvidenceSpec{}, contract.Errorf(contract.CodeUsage, "invalid evidence range in %q", s)
	}
	return EvidenceSpec{Path: path, Start: start, End: end}, nil
}

func parseRange(rng string) (int, int, error) {
	if dash := strings.IndexByte(rng, '-'); dash >= 0 {
		start, err1 := strconv.Atoi(rng[:dash])
		end, err2 := strconv.Atoi(rng[dash+1:])
		if err1 != nil || err2 != nil || start <= 0 || end < start {
			return 0, 0, fmt.Errorf("bad range")
		}
		return start, end, nil
	}
	n, err := strconv.Atoi(rng)
	if err != nil || n <= 0 {
		return 0, 0, fmt.Errorf("bad line")
	}
	return n, n, nil
}

// LinkSpec is a parsed --link value.
type LinkSpec struct {
	Type string // "file" | "symbol"
	Ref  string
}

// ParseLink parses "file:<path>" or "symbol:<name>".
func ParseLink(s string) (LinkSpec, error) {
	typ, ref, ok := strings.Cut(s, ":")
	if !ok || ref == "" || (typ != "file" && typ != "symbol") {
		return LinkSpec{}, contract.Errorf(contract.CodeUsage, "invalid link %q (want file:<path> or symbol:<name>)", s)
	}
	return LinkSpec{Type: typ, Ref: ref}, nil
}

// Manager performs memory operations against a store.
type Manager struct {
	DB      *store.DB
	Clock   clock.Clock
	WorkDir string
}

func (m *Manager) now() string {
	return m.Clock.Now().UTC().Format("2006-01-02T15:04:05Z")
}

// blobOIDOf computes the current git blob oid of a working-tree file, or ""
// when the file cannot be read.
func (m *Manager) blobOIDOf(path string) string {
	content, err := os.ReadFile(filepath.Join(m.WorkDir, filepath.FromSlash(path)))
	if err != nil {
		return ""
	}
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(content))
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}

// resolveLinkWarning returns a warning string if a link target cannot be
// resolved against the current index (links are stored regardless).
func (m *Manager) resolveLinkWarning(l LinkSpec) string {
	switch l.Type {
	case "file":
		if _, ok, _ := m.DB.FileByPath(l.Ref); !ok {
			return fmt.Sprintf("link file:%s does not resolve to an indexed file (stored anyway)", l.Ref)
		}
	case "symbol":
		name := symbolName(l.Ref)
		if rows, _ := m.DB.SymbolsByName(name); len(rows) == 0 {
			return fmt.Sprintf("link symbol:%s does not resolve to an indexed symbol (stored anyway)", l.Ref)
		}
	}
	return ""
}

// symbolName strips a qualifier (Container.name or name@path) to the bare name.
func symbolName(ref string) string {
	if i := strings.IndexByte(ref, '@'); i >= 0 {
		ref = ref[:i]
	}
	if i := strings.LastIndexByte(ref, '.'); i >= 0 {
		ref = ref[i+1:]
	}
	return ref
}

func notFound(id string) *contract.Error {
	return &contract.Error{Code: contract.CodeNotFound, Message: "memory not found: " + id}
}
