package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/orafaelfragoso/columbus/internal/contract"
	"github.com/orafaelfragoso/columbus/internal/store"
)

// ExportSchemaVersion is the version of the export document format. v4
// carries memories only. Import is strict: only v4 documents are accepted.
const ExportSchemaVersion = 4

// ExportEvidence is a portable evidence anchor (keeps the creation blob oid so
// drift detection survives a restore).
type ExportEvidence struct {
	Path              string `json:"path"`
	LineStart         int    `json:"line_start"`
	LineEnd           int    `json:"line_end"`
	BlobOIDAtCreation string `json:"blob_oid_at_creation,omitempty"`
}

// ExportLink is a portable link.
type ExportLink struct {
	TargetType string `json:"target_type"`
	TargetRef  string `json:"target_ref"`
}

// ExportRecord is one memory in the export document.
type ExportRecord struct {
	ID        string           `json:"id"`
	Kind      string           `json:"kind"`
	Title     string           `json:"title"`
	Body      string           `json:"body,omitempty"`
	Tags      []string         `json:"tags,omitempty"`
	Evidence  []ExportEvidence `json:"evidence,omitempty"`
	Links     []ExportLink     `json:"links,omitempty"`
	CreatedAt string           `json:"created_at,omitempty"`
	UpdatedAt string           `json:"updated_at,omitempty"`
}

// ExportDoc is the schema-versioned memory export document.
type ExportDoc struct {
	SchemaVersion int            `json:"schema_version"`
	Memories      []ExportRecord `json:"memories"`
}

// Export gathers memories (optionally filtered by kind/tag) into a portable,
// schema-versioned document.
func (m *Manager) Export(kind, tag string) (ExportDoc, error) {
	if kind != "" && !validKind(kind) {
		return ExportDoc{}, &contract.Error{Code: contract.CodeInvalidKind, Message: "unknown memory kind: " + kind}
	}
	briefs, err := m.DB.ListMemories(kind, tag)
	if err != nil {
		return ExportDoc{}, err
	}
	doc := ExportDoc{SchemaVersion: ExportSchemaVersion}
	for _, b := range briefs {
		full, ok, err := m.DB.MemoryFull(b.ID)
		if err != nil {
			return ExportDoc{}, err
		}
		if !ok {
			continue
		}
		doc.Memories = append(doc.Memories, exportRecordFrom(full))
	}
	return doc, nil
}

// ImportResult is the typed result of import.
type ImportResult struct {
	Total       int  `json:"total"`
	Imported    int  `json:"imported"`
	Skipped     int  `json:"skipped"`
	PreserveIDs bool `json:"preserve_ids"`
}

func (ImportResult) CommandName() string { return "memory" }

// Import merges a memory export document. Only the current schema version is
// accepted — there is no legacy-document fallback. Default mode reassigns
// fresh local ids and dedupes content-duplicate memories. preserveIDs restores
// original ids and errors on any id collision.
func (m *Manager) Import(doc ExportDoc, preserveIDs bool) (ImportResult, error) {
	if doc.SchemaVersion != ExportSchemaVersion {
		return ImportResult{}, &contract.Error{Code: contract.CodeConfigInvalid,
			Message: fmt.Sprintf("unsupported export schema v%d (this version reads only v%d)", doc.SchemaVersion, ExportSchemaVersion),
			Hint:    "re-export with the same columbus version"}
	}

	res := ImportResult{Total: len(doc.Memories), PreserveIDs: preserveIDs}

	// The content-hash->id index is read up front (a bulk read). Preserve-ids
	// collision checks read through the tx instead, so they are both atomic and
	// free of the single-connection deadlock a pool read inside WithTx causes.
	existing, err := m.existingHashIDs()
	if err != nil {
		return ImportResult{}, err
	}

	err = m.DB.WithTx(func(tx *store.Tx) error {
		var maxMem int64
		if err := m.importMemories(tx, doc.Memories, preserveIDs, existing, &res, &maxMem); err != nil {
			return err
		}
		if preserveIDs && maxMem > 0 {
			return tx.SetMemSeqAtLeast(maxMem)
		}
		return nil
	})
	if err != nil {
		return ImportResult{}, err
	}
	return res, nil
}

func (m *Manager) importMemories(tx *store.Tx, recs []ExportRecord, preserveIDs bool, existing map[string]int64, res *ImportResult, maxMem *int64) error {
	for _, rec := range recs {
		if preserveIDs {
			id, err := ParseID(rec.ID)
			if err != nil {
				return err
			}
			exists, err := tx.MemoryExists(id)
			if err != nil {
				return err
			}
			if exists {
				return collision("memory", rec.ID)
			}
			if err := writeRecord(tx, id, rec); err != nil {
				return err
			}
			bump(maxMem, id)
			res.Imported++
			continue
		}
		h := recordHash(rec)
		if _, ok := existing[h]; ok {
			res.Skipped++
			continue
		}
		id, err := tx.NextMemSeq()
		if err != nil {
			return err
		}
		if err := writeRecord(tx, id, rec); err != nil {
			return err
		}
		existing[h] = id
		res.Imported++
	}
	return nil
}

func bump(max *int64, v int64) {
	if v > *max {
		*max = v
	}
}

func collision(kind, id string) *contract.Error {
	return &contract.Error{Code: contract.CodeStoreError,
		Message: "id collision importing " + kind + " " + id + " with --preserve-ids",
		Hint:    "import into an empty store or drop --preserve-ids"}
}

func writeRecord(tx *store.Tx, id int64, rec ExportRecord) error {
	now := rec.CreatedAt
	upd := rec.UpdatedAt
	if err := tx.InsertMemory(id, rec.Kind, rec.Title, rec.Body, now, upd); err != nil {
		return err
	}
	for _, tag := range rec.Tags {
		if err := tx.AddTag(id, tag); err != nil {
			return err
		}
	}
	for _, ev := range rec.Evidence {
		if err := tx.AddEvidence(id, ev.Path, ev.LineStart, ev.LineEnd, ev.BlobOIDAtCreation); err != nil {
			return err
		}
	}
	for _, l := range rec.Links {
		if err := tx.AddLink(id, l.TargetType, l.TargetRef); err != nil {
			return err
		}
	}
	return tx.ReindexMemoryFTS(id, rec.Title, rec.Body, rec.Tags)
}

// existingHashIDs maps each existing memory's content hash to its id, so a
// reassign import can skip content-duplicates.
func (m *Manager) existingHashIDs() (map[string]int64, error) {
	ids, err := m.DB.AllMemoryIDs()
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for _, id := range ids {
		full, ok, err := m.DB.MemoryFull(id)
		if err != nil {
			return nil, err
		}
		if ok {
			out[recordHash(exportRecordFrom(full))] = id
		}
	}
	return out, nil
}

// recordHash is a content hash over the meaningful fields (ignoring id and
// timestamps) for idempotent dedupe.
func recordHash(r ExportRecord) string {
	tags := append([]string(nil), r.Tags...)
	sort.Strings(tags)

	evid := make([]string, 0, len(r.Evidence))
	for _, e := range r.Evidence {
		evid = append(evid, fmt.Sprintf("%s:%d-%d", e.Path, e.LineStart, e.LineEnd))
	}
	sort.Strings(evid)

	links := make([]string, 0, len(r.Links))
	for _, l := range r.Links {
		links = append(links, l.TargetType+":"+l.TargetRef)
	}
	sort.Strings(links)

	canonical := strings.Join([]string{
		r.Kind, r.Title, r.Body,
		strings.Join(tags, ","),
		strings.Join(evid, ","),
		strings.Join(links, ","),
	}, "\x00")
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}

func exportRecordFrom(m store.Memory) ExportRecord {
	rec := ExportRecord{
		ID: FormatID(m.ID), Kind: m.Kind, Title: m.Title, Body: m.Body,
		Tags: m.Tags, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
	for _, e := range m.Evidence {
		rec.Evidence = append(rec.Evidence, ExportEvidence{
			Path: e.Path, LineStart: e.LineStart, LineEnd: e.LineEnd, BlobOIDAtCreation: e.BlobOIDAtCreation,
		})
	}
	for _, l := range m.Links {
		rec.Links = append(rec.Links, ExportLink{TargetType: l.TargetType, TargetRef: l.TargetRef})
	}
	return rec
}
