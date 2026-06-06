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

// ExportSchemaVersion is the version of the export document format. v2 added
// the structured-memory entities (epics and tasks) to the unified knowledge
// document; v1 documents (memories only) still import.
const ExportSchemaVersion = 2

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

// ExportDoc is the schema-versioned unified knowledge document.
type ExportDoc struct {
	SchemaVersion int            `json:"schema_version"`
	Memories      []ExportRecord `json:"memories"`
	Epics         []ExportEpic   `json:"epics,omitempty"`
	Tasks         []ExportTask   `json:"tasks,omitempty"`
}

// Export gathers memories (optionally filtered by kind/tag) plus all epics and
// tasks into a portable, schema-versioned knowledge document. The kind/tag
// filters apply only to memories; epics and tasks are always exported in full.
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
	if doc.Epics, err = m.exportEpics(); err != nil {
		return ExportDoc{}, err
	}
	if doc.Tasks, err = m.exportTasks(); err != nil {
		return ExportDoc{}, err
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

// Import merges a unified knowledge document. Default mode reassigns fresh
// local ids (deduping content-duplicate memories) and fixes up cross-entity
// references so a task/epic memory ref points at the remapped memory.
// preserveIDs restores original ids and errors on any id collision.
func (m *Manager) Import(doc ExportDoc, preserveIDs bool) (ImportResult, error) {
	if doc.SchemaVersion > ExportSchemaVersion {
		return ImportResult{}, &contract.Error{Code: contract.CodeConfigInvalid,
			Message: fmt.Sprintf("export schema v%d is newer than supported v%d", doc.SchemaVersion, ExportSchemaVersion)}
	}

	res := ImportResult{Total: len(doc.Memories) + len(doc.Epics) + len(doc.Tasks), PreserveIDs: preserveIDs}

	// The content-hash->id index is read up front (a bulk read). Preserve-ids
	// collision checks read through the tx instead, so they are both atomic and
	// free of the single-connection deadlock a pool read inside WithTx causes.
	existing, err := m.existingHashIDs()
	if err != nil {
		return ImportResult{}, err
	}

	err = m.DB.WithTx(func(tx *store.Tx) error {
		// memMap/epicMap translate a document's original numeric ids to the ids
		// actually written, so later entities can fix up their cross-references.
		memMap := map[int64]int64{}
		epicMap := map[int64]int64{}
		maxMem, maxEpic, maxTask := int64(0), int64(0), int64(0)

		if err := m.importMemories(tx, doc.Memories, preserveIDs, existing, memMap, &res, &maxMem); err != nil {
			return err
		}
		if err := importEpics(tx, doc.Epics, preserveIDs, memMap, epicMap, &res, &maxEpic); err != nil {
			return err
		}
		if err := importTasks(tx, doc.Tasks, preserveIDs, memMap, epicMap, &res, &maxTask); err != nil {
			return err
		}

		if preserveIDs {
			if err := advanceSeqs(tx, maxMem, maxEpic, maxTask); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ImportResult{}, err
	}
	return res, nil
}

func (m *Manager) importMemories(tx *store.Tx, recs []ExportRecord, preserveIDs bool, existing map[string]int64, memMap map[int64]int64, res *ImportResult, maxMem *int64) error {
	for _, rec := range recs {
		old, _ := ParseID(rec.ID) // 0 if absent/malformed: simply not remapped
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
			memMap[id] = id
			bump(maxMem, id)
			res.Imported++
			continue
		}
		h := recordHash(rec)
		if eid, ok := existing[h]; ok {
			memMap[old] = eid
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
		memMap[old] = id
		res.Imported++
	}
	return nil
}

func importEpics(tx *store.Tx, recs []ExportEpic, preserveIDs bool, memMap, epicMap map[int64]int64, res *ImportResult, maxEpic *int64) error {
	for _, rec := range recs {
		old, _ := parseWorkID(rec.ID, "epic_")
		var id int64
		if preserveIDs {
			parsed, err := parseWorkID(rec.ID, "epic_")
			if err != nil {
				return err
			}
			exists, err := tx.EpicExists(parsed)
			if err != nil {
				return err
			}
			if exists {
				return collision("epic", rec.ID)
			}
			id = parsed
			bump(maxEpic, parsed)
		} else {
			newID, err := tx.NextEpicSeq()
			if err != nil {
				return err
			}
			id = newID
		}
		if err := writeEpic(tx, id, rec, memMap, !preserveIDs); err != nil {
			return err
		}
		epicMap[old] = id
		res.Imported++
	}
	return nil
}

func importTasks(tx *store.Tx, recs []ExportTask, preserveIDs bool, memMap, epicMap map[int64]int64, res *ImportResult, maxTask *int64) error {
	for _, rec := range recs {
		oldEpic, err := parseWorkID(rec.Epic, "epic_")
		if err != nil {
			return err
		}
		if preserveIDs {
			id, err := parseWorkID(rec.ID, "task_")
			if err != nil {
				return err
			}
			exists, err := tx.TaskExists(id)
			if err != nil {
				return err
			}
			if exists {
				return collision("task", rec.ID)
			}
			if err := writeTask(tx, id, oldEpic, rec, memMap, false); err != nil {
				return err
			}
			bump(maxTask, id)
			res.Imported++
			continue
		}
		newEpic, ok := epicMap[oldEpic]
		if !ok {
			return &contract.Error{Code: contract.CodeConfigInvalid,
				Message: "task " + rec.ID + " references epic " + rec.Epic + " not present in the document"}
		}
		id, err := tx.NextTaskSeq()
		if err != nil {
			return err
		}
		if err := writeTask(tx, id, newEpic, rec, memMap, true); err != nil {
			return err
		}
		res.Imported++
	}
	return nil
}

func advanceSeqs(tx *store.Tx, maxMem, maxEpic, maxTask int64) error {
	if maxMem > 0 {
		if err := tx.SetMemSeqAtLeast(maxMem); err != nil {
			return err
		}
	}
	if maxEpic > 0 {
		if err := tx.SetEpicSeqAtLeast(maxEpic); err != nil {
			return err
		}
	}
	if maxTask > 0 {
		if err := tx.SetTaskSeqAtLeast(maxTask); err != nil {
			return err
		}
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
// reassign import can both skip content-duplicates and remap references to the
// already-present memory.
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
