// Package config loads, validates and writes the local-only .columbus.json
// file and resolves the OS data directory. It implements the precedence chain:
// built-in defaults -> file -> environment -> flags (flags/env applied by
// callers on top of a loaded Config).
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"

	"github.com/rafaelfragoso/columbus/internal/contract"
)

// FileName is the local config file name.
const FileName = ".columbus.json"

// SchemaVersion is the highest .columbus.json schema this binary understands.
const SchemaVersion = 1

// DefaultMaxFileSize is the default per-file size ceiling (1.5 MB).
const DefaultMaxFileSize int64 = 1_572_864

// Config is the parsed .columbus.json.
type Config struct {
	SchemaVersion int            `json:"schema_version"`
	ProjectID     string         `json:"project_id"`
	Indexing      IndexingConfig `json:"indexing"`
}

// IndexingConfig holds the indexing knobs.
type IndexingConfig struct {
	Include     []string        `json:"include,omitempty"`
	Exclude     []string        `json:"exclude,omitempty"`
	MaxFileSize int64           `json:"max_file_size,omitempty"`
	Languages   map[string]bool `json:"languages,omitempty"`
}

// Default returns the built-in default configuration (project_id empty; set at
// init).
func Default() Config {
	return Config{
		SchemaVersion: SchemaVersion,
		Indexing: IndexingConfig{
			Include: []string{},
			Exclude: []string{
				"**/node_modules/**",
				"**/.git/**",
				"**/dist/**",
				"**/build/**",
				"**/vendor/**",
				"**/.next/**",
				"**/coverage/**",
			},
			MaxFileSize: DefaultMaxFileSize,
			Languages:   map[string]bool{},
		},
	}
}

// LoadResult carries the parsed config plus any non-fatal warnings (e.g.
// unknown keys, for forward compatibility).
type LoadResult struct {
	Config   Config
	Warnings []string
}

// knownTopLevelKeys is the set of recognised top-level keys; anything else is a
// forward-compat warning.
var knownTopLevelKeys = map[string]bool{
	"schema_version": true,
	"project_id":     true,
	"indexing":       true,
}

// Load reads and validates the config at path. A missing file is
// NOT_INITIALIZED; malformed JSON or invalid values are CONFIG_INVALID; unknown
// keys are warnings, not errors. Omitted fields fall back to defaults.
func Load(path string) (LoadResult, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return LoadResult{}, &contract.Error{
				Code:    contract.CodeNotInitialized,
				Message: "no .columbus.json found",
				Hint:    "run columbus init",
			}
		}
		return LoadResult{}, &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
	}

	// First pass: detect unknown top-level keys for warnings.
	var rawKeys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawKeys); err != nil {
		return LoadResult{}, &contract.Error{
			Code:    contract.CodeConfigInvalid,
			Message: "invalid .columbus.json: " + err.Error(),
		}
	}
	var warnings []string
	for k := range rawKeys {
		if !knownTopLevelKeys[k] {
			warnings = append(warnings, "unknown config key ignored: "+k)
		}
	}

	// Second pass: decode into defaults so omitted fields keep their defaults.
	cfg := Default()
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&cfg); err != nil {
		return LoadResult{}, &contract.Error{
			Code:    contract.CodeConfigInvalid,
			Message: "invalid .columbus.json: " + err.Error(),
		}
	}

	if err := validate(cfg); err != nil {
		return LoadResult{}, err
	}
	return LoadResult{Config: cfg, Warnings: warnings}, nil
}

func validate(c Config) error {
	if c.SchemaVersion < 1 {
		return &contract.Error{Code: contract.CodeConfigInvalid, Message: "schema_version must be >= 1"}
	}
	if c.SchemaVersion > SchemaVersion {
		return &contract.Error{
			Code:    contract.CodeConfigInvalid,
			Message: "config schema_version is newer than this binary supports",
			Hint:    "upgrade columbus",
		}
	}
	if c.ProjectID == "" {
		return &contract.Error{Code: contract.CodeConfigInvalid, Message: "project_id is required"}
	}
	if c.Indexing.MaxFileSize < 0 {
		return &contract.Error{Code: contract.CodeConfigInvalid, Message: "indexing.max_file_size must be >= 0"}
	}
	return nil
}

// Save writes the config to path as pretty-printed JSON (this file is
// human-editable, so prettiness matters; it is not the machine contract).
func Save(path string, c Config) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(c); err != nil {
		return &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
	}
	return nil
}
