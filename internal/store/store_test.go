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
	dsn := "file:" + path + "?_foreign_keys=on"
	raw, err := sql.Open("sqlite3", dsn)
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
	if err := db.UpsertVector("file", 1, "m", "sha", vec384(1, 0, 0)); err != nil {
		t.Fatalf("upsert after upgrade: %v", err)
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
