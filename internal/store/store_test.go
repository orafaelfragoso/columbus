package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/orafaelfragoso/columbus/internal/contract"
)

func openTemp(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "columbus.sqlite")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenAppliesMigrations(t *testing.T) {
	db := openTemp(t)
	var uv int
	if err := db.SQL().QueryRow("PRAGMA user_version").Scan(&uv); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if uv != LatestVersion {
		t.Errorf("user_version = %d, want %d", uv, LatestVersion)
	}
}

func TestWALModeEnabled(t *testing.T) {
	db := openTemp(t)
	var mode string
	if err := db.SQL().QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
}

func TestForeignKeysEnabled(t *testing.T) {
	db := openTemp(t)
	var fk int
	if err := db.SQL().QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("read foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}
}

func TestModernSQLiteDriverProvidesVec0(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raw.sqlite")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open raw sqlite driver: %v", err)
	}
	defer raw.Close()

	var version string
	if err := raw.QueryRow(`SELECT vec_version()`).Scan(&version); err != nil {
		t.Fatalf("vec0 should be available on sqlite driver connections: %v", err)
	}
	if version == "" {
		t.Fatal("vec_version() returned empty version")
	}
}

func TestReopenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "columbus.sqlite")
	db1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	db1.Close()

	db2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer db2.Close()
	var uv int
	if err := db2.SQL().QueryRow("PRAGMA user_version").Scan(&uv); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if uv != LatestVersion {
		t.Errorf("user_version after reopen = %d, want %d", uv, LatestVersion)
	}
}

// TestMigrate0003OnExisting0002DB builds a DB at schema v2 (no embedding
// layer), then opens it and verifies 0003 applies cleanly on top.
func TestMigrate0003OnExisting0002DB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "columbus.sqlite")
	dsn := "file:" + path + "?_pragma=foreign_keys(1)"
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	migs, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, m := range migs {
		if m.version <= 2 {
			if err := applyMigration(raw, m); err != nil {
				t.Fatalf("apply %s: %v", m.name, err)
			}
		}
	}
	raw.Close()

	db, err := Open(path) // applies 0003
	if err != nil {
		t.Fatalf("Open over v2 DB: %v", err)
	}
	defer db.Close()

	var uv int
	if err := db.SQL().QueryRow("PRAGMA user_version").Scan(&uv); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if uv != LatestVersion {
		t.Errorf("user_version = %d, want %d", uv, LatestVersion)
	}
	// vec0 table and the new index_meta columns must now exist and be usable.
	if err := db.UpsertVector("file", 1, "m", "sha", vec256(1, 0, 0)); err != nil {
		t.Fatalf("upsert after upgrade: %v", err)
	}
}

// TestMigrate0005RebuildsVectorLayerForPotion builds a DB with the legacy
// 384-d embedding table, then opens it and verifies the migration clears stale
// BGE vectors and recreates vec0 for the 256-d potion-code model.
func TestMigrate0005RebuildsVectorLayerForPotion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "columbus.sqlite")
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	migs, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, m := range migs {
		if m.version <= 4 {
			if err := applyMigration(raw, m); err != nil {
				t.Fatalf("apply %s: %v", m.name, err)
			}
		}
	}
	raw.Close()

	raw, err = sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("reopen legacy raw: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO chunk_meta (rowid, owner_type, owner_id, model, content_sha)
		VALUES (1, 'file', 1, 'bge-small-en-v1.5', 'old-sha')`); err != nil {
		t.Fatalf("seed legacy chunk_meta: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO vec_chunks (rowid, embedding) VALUES (1, ?)`, serializeFloat32(vec384(1, 0, 0))); err != nil {
		t.Fatalf("seed legacy vec_chunks: %v", err)
	}
	if _, err := raw.Exec(`UPDATE index_meta SET embed_model = 'bge-small-en-v1.5', embed_dim = 384 WHERE id = 1`); err != nil {
		t.Fatalf("seed legacy embed info: %v", err)
	}
	raw.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open over v4 DB: %v", err)
	}
	defer db.Close()

	var chunks int
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM chunk_meta`).Scan(&chunks); err != nil {
		t.Fatalf("count chunk_meta: %v", err)
	}
	if chunks != 0 {
		t.Fatalf("chunk_meta rows after model migration = %d, want 0", chunks)
	}
	meta, err := db.Meta().Get()
	if err != nil {
		t.Fatalf("meta get: %v", err)
	}
	if meta.EmbedModel != "" || meta.EmbedDim != 0 {
		t.Fatalf("embed info after model migration = %q/%d, want empty", meta.EmbedModel, meta.EmbedDim)
	}
	if err := db.UpsertVector("file", 2, "minishlab/potion-code-16M", "new-sha", vec256(1, 0, 0)); err != nil {
		t.Fatalf("upsert 256-d vector after migration: %v", err)
	}
	if err := db.UpsertVector("file", 3, "bge-small-en-v1.5", "bad-sha", vec384(1, 0, 0)); err == nil {
		t.Fatal("upsert 384-d legacy vector succeeded; want vec0 dimension error")
	}
}

func TestFTS5IsAvailable(t *testing.T) {
	db := openTemp(t)
	_, err := db.SQL().Exec(`INSERT INTO code_fts (name, signature, path, package, grain, ref_id)
		VALUES ('parseConfig', 'func parseConfig() error', 'internal/config/config.go', 'config', 'symbol', '7')`)
	if err != nil {
		t.Fatalf("insert into code_fts: %v", err)
	}
	var got string
	err = db.SQL().QueryRow(`SELECT name FROM code_fts WHERE code_fts MATCH 'parseConfig'`).Scan(&got)
	if err != nil {
		t.Fatalf("fts match query: %v", err)
	}
	if got != "parseConfig" {
		t.Errorf("fts match = %q", got)
	}
}

func TestSchemaTooNew(t *testing.T) {
	path := filepath.Join(t.TempDir(), "columbus.sqlite")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := db.SQL().Exec("PRAGMA user_version = 9999"); err != nil {
		t.Fatalf("bump user_version: %v", err)
	}
	db.Close()

	_, err = Open(path)
	if err == nil {
		t.Fatal("expected SCHEMA_TOO_NEW error")
	}
	var ce *contract.Error
	if !errors.As(err, &ce) || ce.Code != contract.CodeSchemaTooNew {
		t.Errorf("err = %v, want SCHEMA_TOO_NEW", err)
	}
}

func TestMetaRepoProjectIDRoundTrip(t *testing.T) {
	db := openTemp(t)
	meta := db.Meta()

	got, err := meta.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ProjectID != "" {
		t.Errorf("fresh project_id = %q, want empty", got.ProjectID)
	}

	if err := meta.SetProjectID("proj_abc123"); err != nil {
		t.Fatalf("SetProjectID: %v", err)
	}
	got, err = meta.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ProjectID != "proj_abc123" {
		t.Errorf("project_id = %q", got.ProjectID)
	}
}

func TestNextMemSeqIsMonotonic(t *testing.T) {
	db := openTemp(t)
	meta := db.Meta()
	var seen []int64
	for range 5 {
		n, err := meta.NextMemSeq()
		if err != nil {
			t.Fatalf("NextMemSeq: %v", err)
		}
		seen = append(seen, n)
	}
	want := []int64{1, 2, 3, 4, 5}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("seq = %v, want %v", seen, want)
		}
	}
}

func TestTxMemoryExistsReadsInsideTransaction(t *testing.T) {
	db := openTemp(t)

	// A read inside WithTx must use the transaction's own connection. With
	// SetMaxOpenConns(1) a pool read here would block until busy_timeout and
	// fail; the tx-scoped read both avoids that and sees uncommitted writes.
	err := db.WithTx(func(tx *Tx) error {
		if err := tx.InsertMemory(1, "decision", "t", "b", "now", "now"); err != nil {
			return err
		}
		exists, err := tx.MemoryExists(1)
		if err != nil {
			return err
		}
		if !exists {
			t.Error("tx.MemoryExists(1) = false inside tx, want true (read-your-writes)")
		}
		absent, err := tx.MemoryExists(2)
		if err != nil {
			return err
		}
		if absent {
			t.Error("tx.MemoryExists(2) = true, want false")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithTx: %v", err)
	}
}

func TestWithTxCommitsAndRollsBack(t *testing.T) {
	db := openTemp(t)

	err := db.WithTx(func(tx *Tx) error {
		_, e := tx.SQL().Exec("INSERT INTO files (path) VALUES ('a.go')")
		return e
	})
	if err != nil {
		t.Fatalf("WithTx commit: %v", err)
	}

	wantErr := errors.New("boom")
	err = db.WithTx(func(tx *Tx) error {
		if _, e := tx.SQL().Exec("INSERT INTO files (path) VALUES ('b.go')"); e != nil {
			return e
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("WithTx err = %v, want boom", err)
	}

	var count int
	if err := db.SQL().QueryRow("SELECT COUNT(*) FROM files").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("files count = %d, want 1 (rollback dropped b.go)", count)
	}
}
