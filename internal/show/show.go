// Package show renders detailed views of a single entity: a symbol (all
// matching definitions), a file (outline + graph), or a memory by id. Line
// ranges and snippets are resolved live against the working tree.
package show

import (
	"strconv"
	"strings"

	"github.com/rafaelfragoso/columbus/internal/contract"
	"github.com/rafaelfragoso/columbus/internal/extract"
	"github.com/rafaelfragoso/columbus/internal/live"
	"github.com/rafaelfragoso/columbus/internal/store"
)

// maxSymbolBlocks caps how many definitions `show symbol` prints.
const maxSymbolBlocks = 25

// Shower renders entity views against a store + working tree.
type Shower struct {
	DB       *store.DB
	WorkDir  string
	Registry *extract.Registry
}

func (s *Shower) cache() *live.Cache { return live.New(s.WorkDir, s.Registry) }

// Symbol returns all definitions matching name, optionally narrowed to files
// whose path contains `in`.
func (s *Shower) Symbol(name, in string, contextLines int) (SymbolResult, error) {
	rows, err := s.DB.SymbolsByName(name)
	if err != nil {
		return SymbolResult{}, err
	}
	if in != "" {
		filtered := rows[:0]
		for _, r := range rows {
			if strings.Contains(r.Path, in) {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}
	if len(rows) == 0 {
		suggestions, _ := s.DB.SuggestSymbols(name, 5)
		return SymbolResult{}, notFound("symbol", name, suggestions)
	}

	total := len(rows)
	capped := false
	if len(rows) > maxSymbolBlocks {
		rows = rows[:maxSymbolBlocks]
		capped = true
	}

	cache := s.cache()
	blocks := make([]SymbolBlock, 0, len(rows))
	for _, r := range rows {
		b := SymbolBlock{
			Name: r.Name, Kind: r.Kind, Container: r.Container, Signature: r.Signature,
			Path: r.Path, Package: r.Package, Role: r.Role, Exported: r.Exported,
		}
		if sym, ok := cache.FindSymbol(r.Path, r.Name, r.Container, r.Kind); ok {
			b.StartLine = sym.StartLine
			b.EndLine = sym.EndLine
			b.Snippet = live.Snippet(cache.SourceLines(r.Path), sym.StartLine, sym.EndLine, contextLines)
		}
		b.Tests, _ = s.DB.TestsOf(r.FileID)
		b.Memories = memoryRefs(s.DB, "symbol", r.Name)
		blocks = append(blocks, b)
	}
	return SymbolResult{Query: name, In: in, Total: total, Capped: capped, Blocks: blocks}, nil
}

// File returns a file's outline (symbols + live ranges), graph and memories.
func (s *Shower) File(path string, contextLines int) (FileResult, error) {
	file, ok, err := s.DB.FileByPath(path)
	if err != nil {
		return FileResult{}, err
	}
	if !ok {
		base := path
		if i := strings.LastIndexByte(path, '/'); i >= 0 {
			base = path[i+1:]
		}
		suggestions, _ := s.DB.SuggestPaths(base, 5)
		return FileResult{}, notFound("file", path, suggestions)
	}

	syms, err := s.DB.SymbolsInFile(file.ID)
	if err != nil {
		return FileResult{}, err
	}
	cache := s.cache()
	outline := make([]OutlineEntry, 0, len(syms))
	for _, r := range syms {
		entry := OutlineEntry{Name: r.Name, Kind: r.Kind, Container: r.Container, Signature: r.Signature, Exported: r.Exported}
		if sym, ok := cache.FindSymbol(r.Path, r.Name, r.Container, r.Kind); ok {
			entry.StartLine = sym.StartLine
			entry.EndLine = sym.EndLine
		}
		outline = append(outline, entry)
	}

	imports, _ := s.DB.ImportsOf(file.ID)
	importedBy, _ := s.DB.ImportedBy(file.ID)
	tests, _ := s.DB.TestsOf(file.ID)
	_ = contextLines

	return FileResult{
		Path: file.Path, Language: file.Language, Package: file.Package, Role: file.Role,
		Outline: outline,
		Imports: imports, ImportedBy: importedBy, Tests: tests,
		Memories: memoryRefs(s.DB, "file", file.Path),
	}, nil
}

// Memory returns a memory by its mem_NNN id.
func (s *Shower) Memory(id string) (MemoryResult, error) {
	n, err := ParseMemoryID(id)
	if err != nil {
		return MemoryResult{}, err
	}
	full, ok, err := s.DB.MemoryFull(n)
	if err != nil {
		return MemoryResult{}, err
	}
	if !ok {
		return MemoryResult{}, notFound("memory", id, nil)
	}
	return memoryResultFrom(full), nil
}

// ParseMemoryID parses a mem_NNN identifier into its numeric id.
func ParseMemoryID(id string) (int64, error) {
	raw := strings.TrimPrefix(id, "mem_")
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v <= 0 {
		return 0, contract.Errorf(contract.CodeUsage, "invalid memory id %q (want mem_NNN)", id)
	}
	return v, nil
}

func notFound(kind, ref string, suggestions []string) *contract.Error {
	e := &contract.Error{Code: contract.CodeNotFound, Message: kind + " not found: " + ref}
	if len(suggestions) > 0 {
		e.Hint = "did you mean: " + strings.Join(suggestions, ", ")
	}
	return e
}

func memoryRefs(db *store.DB, targetType, targetRef string) []MemoryRef {
	mems, _ := db.MemoriesForTarget(targetType, targetRef)
	if len(mems) == 0 {
		return nil
	}
	out := make([]MemoryRef, len(mems))
	for i, m := range mems {
		out[i] = MemoryRef{ID: memID(m.ID), Kind: m.Kind, Title: m.Title}
	}
	return out
}
