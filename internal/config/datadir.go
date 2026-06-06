package config

import (
	"path/filepath"
	"runtime"

	"github.com/rafaelfragoso/columbus/internal/contract"
)

// DataDir resolves the Columbus data directory. COLUMBUS_DATA_DIR overrides
// everything (essential for tests); otherwise OS conventions apply.
//
//	Linux:   $XDG_DATA_HOME/columbus or ~/.local/share/columbus
//	macOS:   ~/Library/Application Support/columbus
//	Windows: %LocalAppData%\columbus
func DataDir(getenv func(string) string) (string, error) {
	if override := getenv("COLUMBUS_DATA_DIR"); override != "" {
		return override, nil
	}

	switch runtime.GOOS {
	case "darwin":
		home := getenv("HOME")
		if home == "" {
			return "", missingHome()
		}
		return filepath.Join(home, "Library", "Application Support", "columbus"), nil
	case "windows":
		if la := getenv("LocalAppData"); la != "" {
			return filepath.Join(la, "columbus"), nil
		}
		return "", &contract.Error{Code: contract.CodeStoreError, Message: "cannot resolve %LocalAppData%"}
	default:
		if xdg := getenv("XDG_DATA_HOME"); xdg != "" {
			return filepath.Join(xdg, "columbus"), nil
		}
		home := getenv("HOME")
		if home == "" {
			return "", missingHome()
		}
		return filepath.Join(home, ".local", "share", "columbus"), nil
	}
}

func missingHome() error {
	return &contract.Error{
		Code:    contract.CodeStoreError,
		Message: "cannot resolve home directory",
		Hint:    "set COLUMBUS_DATA_DIR",
	}
}

// Paths holds the per-project storage locations under the data directory.
type Paths struct {
	ProjectDir string
	DBPath     string
	LogPath    string
	ExportsDir string
}

// ProjectPaths returns the storage paths for a project under dataDir.
func ProjectPaths(dataDir, projectID string) Paths {
	dir := filepath.Join(dataDir, "projects", projectID)
	return Paths{
		ProjectDir: dir,
		DBPath:     filepath.Join(dir, "columbus.sqlite"),
		LogPath:    filepath.Join(dir, "logs.jsonl"),
		ExportsDir: filepath.Join(dir, "exports"),
	}
}
