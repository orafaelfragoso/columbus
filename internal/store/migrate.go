package store

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/orafaelfragoso/columbus/internal/contract"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// migration is one ordered, embedded migration keyed to a user_version.
type migration struct {
	version int
	name    string
	sql     string
}

// loadMigrations parses and orders the embedded migration files. File names
// must be NNNN_description.sql.
func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, err
	}
	var out []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		prefix, _, ok := strings.Cut(e.Name(), "_")
		if !ok {
			return nil, fmt.Errorf("migration %q missing NNNN_ prefix", e.Name())
		}
		v, err := strconv.Atoi(prefix)
		if err != nil {
			return nil, fmt.Errorf("migration %q has non-numeric prefix: %w", e.Name(), err)
		}
		body, err := migrationFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, migration{version: v, name: e.Name(), sql: string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

// LatestVersion is the highest embedded migration version (the schema version
// this binary writes). Computed once at init.
var LatestVersion = func() int {
	migs, err := loadMigrations()
	if err != nil {
		panic("store: cannot load migrations: " + err.Error())
	}
	if len(migs) == 0 {
		return 0
	}
	return migs[len(migs)-1].version
}()

// migrate applies all pending migrations in order. Each runs in its own
// transaction; user_version is advanced after each. A database newer than this
// binary fails with SCHEMA_TOO_NEW.
func migrate(db *sql.DB) error {
	migs, err := loadMigrations()
	if err != nil {
		return &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
	}

	var current int
	if err := db.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
	}

	if current > LatestVersion {
		return &contract.Error{
			Code:    contract.CodeSchemaTooNew,
			Message: fmt.Sprintf("database schema v%d is newer than this binary (v%d)", current, LatestVersion),
			Hint:    "upgrade columbus",
		}
	}

	for _, m := range migs {
		if m.version <= current {
			continue
		}
		if err := applyMigration(db, m); err != nil {
			return err
		}
	}
	return nil
}

func applyMigration(db *sql.DB, m migration) error {
	tx, err := db.Begin()
	if err != nil {
		return &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
	}
	if _, err := tx.Exec(m.sql); err != nil {
		_ = tx.Rollback()
		return &contract.Error{Code: contract.CodeStoreError, Message: fmt.Sprintf("migration %s: %v", m.name, err)}
	}
	// PRAGMA user_version does not accept bound parameters.
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", m.version)); err != nil {
		_ = tx.Rollback()
		return &contract.Error{Code: contract.CodeStoreError, Message: fmt.Sprintf("set user_version %d: %v", m.version, err)}
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
	}
	return nil
}
