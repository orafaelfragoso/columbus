package tui

import (
	"fmt"
	"strings"

	"github.com/orafaelfragoso/columbus/internal/memory"
	"github.com/orafaelfragoso/columbus/internal/store"
)

// StoreSource loads a Snapshot from the project store and memory manager. It is
// a pure read: it never mutates and never re-resolves the working tree.
type StoreSource struct {
	DB     *store.DB
	Memory *memory.Manager
	Branch string // resolved by the caller (git); blank is tolerated
}

// Load reads index metadata, memories and embeddings into an immutable
// Snapshot.
func (s *StoreSource) Load() (Snapshot, error) {
	meta, err := s.DB.Meta().Get()
	if err != nil {
		return Snapshot{}, err
	}

	var memRows []MemRow
	var memCounts map[string]int
	memTotal := 0
	if s.Memory != nil {
		list, err := s.Memory.List("", "")
		if err != nil {
			return Snapshot{}, err
		}
		memCounts, memTotal = list.Counts, list.Total
		memRows = make([]MemRow, 0, len(list.Memories))
		for _, m := range list.Memories {
			memRows = append(memRows, MemRow{ID: m.ID, Kind: m.Kind, Title: m.Title, Tags: m.Tags})
		}
	}

	embeddings, err := s.DB.VectorCount()
	if err != nil {
		return Snapshot{}, err
	}

	return Snapshot{
		Branch: s.Branch, Head: meta.IndexedHead, Dirty: meta.Dirty,
		Files: meta.FilesCount, Symbols: meta.SymbolsCount, Embeddings: embeddings,
		Memories: memTotal, MemCounts: memCounts,
		Mems: memRows,
	}, nil
}

// Detail renders a full markdown document for a memory: body, tags, links.
// Returns "" when the id is unknown.
func (s *StoreSource) Detail(kind string, id int64) (string, error) {
	if kind != "memory" {
		return "", nil
	}
	mm, ok, err := s.DB.MemoryFull(id)
	if err != nil || !ok {
		return "", err
	}
	return memoryDetailMarkdown(mm), nil
}

func memoryDetailMarkdown(mm store.Memory) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", mm.Title)
	fmt.Fprintf(&b, "**Kind:** %s  ·  **ID:** `%s`\n\n", kindLabel(mm.Kind), memory.FormatID(mm.ID))
	if mm.Body != "" {
		b.WriteString(mm.Body + "\n\n")
	}
	if len(mm.Tags) > 0 {
		fmt.Fprintf(&b, "**Tags:** %s\n", strings.Join(mm.Tags, ", "))
	}
	return b.String()
}
