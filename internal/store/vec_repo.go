package store

import (
	"database/sql"
	"strings"

	sqlitevec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

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
	blob, err := sqlitevec.SerializeFloat32(vec)
	if err != nil {
		return storeErr(err)
	}
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
	blob, err := sqlitevec.SerializeFloat32(qvec)
	if err != nil {
		return nil, storeErr(err)
	}

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

// placeholders returns "?, ?, ..." with n marks for an IN clause.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}
