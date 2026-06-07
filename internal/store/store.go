// Package store owns the SQLite database: connection setup, PRAGMAs, embedded
// migrations, and typed repositories. The DB is a metadata + graph cache, never
// a content store.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	sqlite3 "github.com/mattn/go-sqlite3"

	"github.com/orafaelfragoso/columbus/internal/contract"
)

// DB is an open Columbus database.
type DB struct {
	db *sql.DB
}

// Open opens (creating if needed) the database at path, applies PRAGMAs and any
// pending migrations, and returns the wrapper. A DB whose user_version is newer
// than this binary understands fails with SCHEMA_TOO_NEW.
func Open(path string) (*DB, error) {
	// WAL for reader/writer concurrency; immediate tx-lock so writes take the
	// reserved lock up front (writer exclusivity); busy_timeout to wait on
	// contention rather than fail instantly; foreign keys on.
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_synchronous=NORMAL&_foreign_keys=on&_busy_timeout=5000&_txlock=immediate",
		path)
	sqlDB, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
	}
	// Serialize access within the process; our writer is serialized anyway and
	// this avoids self-inflicted lock contention across goroutines.
	sqlDB.SetMaxOpenConns(1)

	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
	}

	db := &DB{db: sqlDB}
	if err := migrate(sqlDB); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return db, nil
}

// SQL exposes the underlying database handle for reads and ad-hoc queries.
func (d *DB) SQL() *sql.DB { return d.db }

// Close closes the database.
func (d *DB) Close() error { return d.db.Close() }

// Meta returns the index-metadata repository.
func (d *DB) Meta() *MetaRepo { return &MetaRepo{db: d.db} }

// Tx is a write transaction. All writes flow through WithTx so the index is
// atomic (all-or-nothing).
type Tx struct {
	tx *sql.Tx
}

// SQL exposes the underlying transaction handle.
func (t *Tx) SQL() *sql.Tx { return t.tx }

// WithTx runs fn inside a single immediate transaction. On error the tx rolls
// back; otherwise it commits. SQLITE_BUSY is surfaced as INDEX_LOCKED.
func (d *DB) WithTx(fn func(*Tx) error) error {
	sqlTx, err := d.db.Begin()
	if err != nil {
		return mapLockErr(err)
	}
	if err := fn(&Tx{tx: sqlTx}); err != nil {
		_ = sqlTx.Rollback()
		return err
	}
	if err := sqlTx.Commit(); err != nil {
		_ = sqlTx.Rollback()
		return mapLockErr(err)
	}
	return nil
}

// mapLockErr converts a SQLite busy/locked error into INDEX_LOCKED; other
// errors become STORE_ERROR (unless already a contract error).
func mapLockErr(err error) error {
	if err == nil {
		return nil
	}
	var ce *contract.Error
	if errors.As(err, &ce) {
		return ce
	}
	var se sqlite3.Error
	if errors.As(err, &se) && (se.Code == sqlite3.ErrBusy || se.Code == sqlite3.ErrLocked) {
		return &contract.Error{
			Code:    contract.CodeIndexLocked,
			Message: "another columbus process holds the writer lock",
			Hint:    "retry once the other operation finishes",
		}
	}
	if strings.Contains(err.Error(), "database is locked") {
		return &contract.Error{Code: contract.CodeIndexLocked, Message: err.Error()}
	}
	return &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
}
