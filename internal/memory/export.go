package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/rafaelfragoso/columbus/internal/contract"
	"github.com/rafaelfragoso/columbus/internal/store"
)

// ExportSchemaVersion is the version of the export document format.
const ExportSchemaVersion = 1

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

// ExportDoc is the schema-versioned export document.
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

// Import merges an export document. Default mode reassigns fresh local ids and
// skips content-duplicate records (idempotent). preserveIDs restores original
// ids into an empty/compatible store and errors on id collision.
func (m *Manager) Import(doc ExportDoc, preserveIDs bool) (ImportResult, error) {
	if doc.SchemaVersion > ExportSchemaVersion {
		return ImportResult{}, &contract.Error{Code: contract.CodeConfigInvalid,
			Message: fmt.Sprintf("export schema v%d is newer than supported v%d", doc.SchemaVersion, ExportSchemaVersion)}
	}

	res := ImportResult{Total: len(doc.Memories), PreserveIDs: preserveIDs}

	// The content-hash dedup set is read up front (a bulk read). The
	// preserve-ids collision check reads through the tx instead (see
	// importPreserve), so it is both atomic and free of the single-connection
	// deadlock that a pool read inside WithTx would cause.
	existing, err := m.existingHashes()
	if err != nil {
		return ImportResult{}, err
	}

	err = m.DB.WithTx(func(tx *store.Tx) error {
		maxID := int64(0)
		for _, rec := range doc.Memories {
			if preserveIDs {
				imported, err := importPreserve(tx, rec)
				if err != nil {
					return err
				}
				if imported > maxID {
					maxID = imported
				}
				res.Imported++
				continue
			}

			h := recordHash(rec)
			if existing[h] {
				res.Skipped++
				continue
			}
			existing[h] = true
			if err := m.importReassign(tx, rec); err != nil {
				return err
			}
			res.Imported++
		}
		if preserveIDs && maxID > 0 {
			return tx.SetMemSeqAtLeast(maxID)
		}
		return nil
	})
	if err != nil {
		return ImportResult{}, err
	}
	return res, nil
}

func (m *Manager) importReassign(tx *store.Tx, rec ExportRecord) error {
	id, err := tx.NextMemSeq()
	if err != nil {
		return err
	}
	return writeRecord(tx, id, rec)
}

func importPreserve(tx *store.Tx, rec ExportRecord) (int64, error) {
	id, err := ParseID(rec.ID)
	if err != nil {
		return 0, err
	}
	exists, err := tx.MemoryExists(id)
	if err != nil {
		return 0, err
	}
	if exists {
		return 0, &contract.Error{Code: contract.CodeStoreError,
			Message: "id collision importing " + rec.ID + " with --preserve-ids",
			Hint:    "import into an empty store or drop --preserve-ids"}
	}
	return id, writeRecord(tx, id, rec)
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

func (m *Manager) existingHashes() (map[string]bool, error) {
	ids, err := m.DB.AllMemoryIDs()
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, id := range ids {
		full, ok, err := m.DB.MemoryFull(id)
		if err != nil {
			return nil, err
		}
		if ok {
			out[recordHash(exportRecordFrom(full))] = true
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
