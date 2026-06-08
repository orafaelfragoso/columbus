package store

import (
	"database/sql"
	"encoding/binary"
	"math"
	"strings"
)

// VecVersion returns the registered sqlite-vec (vec0) extension version,
// confirming the vector layer is loadable. Used by `columbus doctor`.
func (d *DB) VecVersion() (string, error) {
	var v string
	if err := d.db.QueryRow(`SELECT vec_version()`).Scan(&v); err != nil {
		return "", storeErr(err)
	}
	return v, nil
}

// VecHit is one nearest-neighbor result from a vector search.
type VecHit struct {
	OwnerType string
	OwnerID   int64
	Distance  float64 // cosine distance; lower = closer
}

// UpsertVector stores or replaces the vector for an owner under the given
// model. The vector blob in vec_chunks and the chunk_meta key row are kept in
// lockstep, keyed by a shared rowid. Re-embedding the same owner under the same
// model overwrites in place. Must run under the writer lock.
func (d *DB) UpsertVector(ownerType string, ownerID int64, model, sha string, vec []float32) error {
	blob := serializeFloat32(vec)
	return d.WithTx(func(tx *Tx) error {
		// Reuse the existing rowid when this owner+model already has a vector so
		// the vec_chunks blob and chunk_meta row stay aligned.
		var rowid int64
		err := tx.tx.QueryRow(
			`SELECT rowid FROM chunk_meta WHERE owner_type = ? AND owner_id = ? AND model = ?`,
			ownerType, ownerID, model).Scan(&rowid)
		switch {
		case err == sql.ErrNoRows:
			res, ierr := tx.tx.Exec(
				`INSERT INTO chunk_meta (owner_type, owner_id, model, content_sha) VALUES (?, ?, ?, ?)`,
				ownerType, ownerID, model, sha)
			if ierr != nil {
				return storeErr(ierr)
			}
			if rowid, ierr = res.LastInsertId(); ierr != nil {
				return storeErr(ierr)
			}
			if _, ierr := tx.tx.Exec(`INSERT INTO vec_chunks (rowid, embedding) VALUES (?, ?)`, rowid, blob); ierr != nil {
				return storeErr(ierr)
			}
		case err != nil:
			return storeErr(err)
		default:
			if _, uerr := tx.tx.Exec(`UPDATE chunk_meta SET content_sha = ? WHERE rowid = ?`, sha, rowid); uerr != nil {
				return storeErr(uerr)
			}
			// vec0 virtual tables don't support UPDATE of the vector column;
			// delete-then-insert at the same rowid replaces it.
			if _, derr := tx.tx.Exec(`DELETE FROM vec_chunks WHERE rowid = ?`, rowid); derr != nil {
				return storeErr(derr)
			}
			if _, ierr := tx.tx.Exec(`INSERT INTO vec_chunks (rowid, embedding) VALUES (?, ?)`, rowid, blob); ierr != nil {
				return storeErr(ierr)
			}
		}
		return nil
	})
}

// SearchVectors returns the nearest owners to qvec, restricted to ownerTypes
// (empty = all types), best (smallest cosine distance) first, capped at k.
func (d *DB) SearchVectors(qvec []float32, ownerTypes []string, k int) ([]VecHit, error) {
	blob := serializeFloat32(qvec)

	// A KNN MATCH must be evaluated on vec_chunks first, then joined to
	// chunk_meta for the polymorphic key and the owner_type filter.
	query := `SELECT m.owner_type, m.owner_id, v.distance
		FROM vec_chunks v JOIN chunk_meta m ON m.rowid = v.rowid
		WHERE v.embedding MATCH ? AND k = ?`
	args := []any{blob, k}
	if len(ownerTypes) > 0 {
		query += ` AND m.owner_type IN (` + placeholders(len(ownerTypes)) + `)`
		for _, t := range ownerTypes {
			args = append(args, t)
		}
	}
	query += ` ORDER BY v.distance`

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, storeErr(err)
	}
	defer rows.Close()

	var out []VecHit
	for rows.Next() {
		var h VecHit
		if err := rows.Scan(&h.OwnerType, &h.OwnerID, &h.Distance); err != nil {
			return nil, storeErr(err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// ChunkSHA returns the stored content_sha for an owner under model, and whether
// a row exists. Callers use this to skip re-embedding unchanged content.
func (d *DB) ChunkSHA(ownerType string, ownerID int64, model string) (string, bool, error) {
	var sha string
	err := d.db.QueryRow(
		`SELECT content_sha FROM chunk_meta WHERE owner_type = ? AND owner_id = ? AND model = ?`,
		ownerType, ownerID, model).Scan(&sha)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, storeErr(err)
	}
	return sha, true, nil
}

// DeleteVectors removes all vectors (every model) for the given owners of a
// type. Companion to the existing cascade cleanup on delete paths; must run
// under the writer lock.
func (d *DB) DeleteVectors(ownerType string, ownerIDs []int64) error {
	if len(ownerIDs) == 0 {
		return nil
	}
	return d.WithTx(func(tx *Tx) error {
		in := placeholders(len(ownerIDs))
		args := make([]any, 0, len(ownerIDs)+1)
		args = append(args, ownerType)
		for _, id := range ownerIDs {
			args = append(args, id)
		}
		// Drop the vec rows first (joined by rowid), then their meta keys.
		if _, err := tx.tx.Exec(
			`DELETE FROM vec_chunks WHERE rowid IN (
				SELECT rowid FROM chunk_meta WHERE owner_type = ? AND owner_id IN (`+in+`))`,
			args...); err != nil {
			return storeErr(err)
		}
		if _, err := tx.tx.Exec(
			`DELETE FROM chunk_meta WHERE owner_type = ? AND owner_id IN (`+in+`)`,
			args...); err != nil {
			return storeErr(err)
		}
		return nil
	})
}

// SymbolChunk is a stored symbol vector with the identity needed to carry it
// forward across a reindex (where the symbol row is deleted and reinserted
// under a new id, but its content — and thus its vector — may be unchanged).
type SymbolChunk struct {
	SymbolID  int64
	Name      string
	Container string
	Signature string
	SHA       string
	Vec       []float32
}

// FileSymbolChunks returns the stored vectors for a file's symbols under model,
// for the index embed phase to decide which symbols can skip re-embedding.
func (d *DB) FileSymbolChunks(fileID int64, model string) ([]SymbolChunk, error) {
	rows, err := d.db.Query(`SELECT s.id, s.name, s.container, s.signature, m.content_sha, v.embedding
		FROM symbols s
		JOIN chunk_meta m ON m.owner_type = 'symbol' AND m.owner_id = s.id AND m.model = ?
		JOIN vec_chunks v ON v.rowid = m.rowid
		WHERE s.file_id = ?`, model, fileID)
	if err != nil {
		return nil, storeErr(err)
	}
	defer rows.Close()
	var out []SymbolChunk
	for rows.Next() {
		var c SymbolChunk
		var blob []byte
		if err := rows.Scan(&c.SymbolID, &c.Name, &c.Container, &c.Signature, &c.SHA, &blob); err != nil {
			return nil, storeErr(err)
		}
		c.Vec = deserializeFloat32(blob)
		out = append(out, c)
	}
	return out, rows.Err()
}

// SymbolIDsByFile returns a file's symbol ids in insertion order (ascending id),
// which aligns with the order symbols were written by PutFile.
func (d *DB) SymbolIDsByFile(fileID int64) ([]int64, error) {
	rows, err := d.db.Query(`SELECT id FROM symbols WHERE file_id = ? ORDER BY id`, fileID)
	if err != nil {
		return nil, storeErr(err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, storeErr(err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ClearVectors drops the entire semantic layer (full/clean reindex). The vec0
// and chunk_meta tables have no FK to files, so a wholesale reindex must clear
// them explicitly.
func (d *DB) ClearVectors() error {
	return d.WithTx(func(tx *Tx) error {
		for _, stmt := range []string{`DELETE FROM vec_chunks`, `DELETE FROM chunk_meta`} {
			if _, err := tx.tx.Exec(stmt); err != nil {
				return storeErr(err)
			}
		}
		return nil
	})
}

// placeholders returns "?, ?, ..." with n marks for an IN clause.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

// deserializeFloat32 decodes a vec0 float32 blob (little-endian, 4 bytes
// per component) back into a slice.
func deserializeFloat32(blob []byte) []float32 {
	v := make([]float32, len(blob)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
	}
	return v
}

func serializeFloat32(v []float32) []byte {
	blob := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(blob[i*4:], math.Float32bits(f))
	}
	return blob
}
