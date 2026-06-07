package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/orafaelfragoso/columbus/internal/contract"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefaultConfigHasSaneIndexing(t *testing.T) {
	c := Default()
	if c.SchemaVersion != SchemaVersion {
		t.Errorf("schema_version = %d", c.SchemaVersion)
	}
	if c.Indexing.MaxFileSize <= 0 {
		t.Errorf("default max_file_size should be positive, got %d", c.Indexing.MaxFileSize)
	}
	if len(c.Indexing.Exclude) == 0 {
		t.Error("default exclude list should not be empty")
	}
}

func TestLoadValidConfig(t *testing.T) {
	path := writeConfig(t, `{
		"schema_version": 1,
		"project_id": "proj_abc12345",
		"indexing": { "max_file_size": 2048, "exclude": ["dist/**"] }
	}`)
	res, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if res.Config.ProjectID != "proj_abc12345" {
		t.Errorf("project_id = %q", res.Config.ProjectID)
	}
	if res.Config.Indexing.MaxFileSize != 2048 {
		t.Errorf("max_file_size = %d", res.Config.Indexing.MaxFileSize)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", res.Warnings)
	}
}

func TestLoadUnknownKeyIsWarningNotError(t *testing.T) {
	path := writeConfig(t, `{
		"schema_version": 1,
		"project_id": "proj_abc12345",
		"future_feature": true
	}`)
	res, err := Load(path)
	if err != nil {
		t.Fatalf("unknown key should not error: %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Error("expected a forward-compat warning for unknown key")
	}
}

func TestLoadInvalidJSONIsConfigInvalid(t *testing.T) {
	path := writeConfig(t, `{ not json `)
	_, err := Load(path)
	assertCode(t, err, contract.CodeConfigInvalid)
}

func TestLoadMissingProjectIDIsInvalid(t *testing.T) {
	path := writeConfig(t, `{ "schema_version": 1 }`)
	_, err := Load(path)
	assertCode(t, err, contract.CodeConfigInvalid)
}

func TestLoadNegativeMaxFileSizeIsInvalid(t *testing.T) {
	path := writeConfig(t, `{
		"schema_version": 1, "project_id": "proj_abc12345",
		"indexing": { "max_file_size": -5 }
	}`)
	_, err := Load(path)
	assertCode(t, err, contract.CodeConfigInvalid)
}

func TestLoadFutureSchemaVersionIsInvalid(t *testing.T) {
	path := writeConfig(t, `{ "schema_version": 999, "project_id": "proj_abc12345" }`)
	_, err := Load(path)
	assertCode(t, err, contract.CodeConfigInvalid)
}

func TestLoadMissingFileIsNotInitialized(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	assertCode(t, err, contract.CodeNotInitialized)
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	c := Default()
	c.ProjectID = "proj_roundtrip01"
	if err := Save(path, c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	res, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if res.Config.ProjectID != "proj_roundtrip01" {
		t.Errorf("project_id = %q", res.Config.ProjectID)
	}
}

func TestLoadAppliesDefaultsForOmittedFields(t *testing.T) {
	path := writeConfig(t, `{ "schema_version": 1, "project_id": "proj_abc12345" }`)
	res, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if res.Config.Indexing.MaxFileSize != Default().Indexing.MaxFileSize {
		t.Errorf("omitted max_file_size should fall back to default, got %d", res.Config.Indexing.MaxFileSize)
	}
}

func assertCode(t *testing.T, err error, want contract.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %s, got nil", want)
	}
	var ce *contract.Error
	if !errors.As(err, &ce) {
		t.Fatalf("error %v is not a *contract.Error", err)
	}
	if ce.Code != want {
		t.Errorf("code = %s, want %s", ce.Code, want)
	}
}
