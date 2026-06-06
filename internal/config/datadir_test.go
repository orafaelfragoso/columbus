package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDataDirHonorsOverride(t *testing.T) {
	getenv := func(k string) string {
		if k == "COLUMBUS_DATA_DIR" {
			return "/tmp/custom-columbus"
		}
		return ""
	}
	got, err := DataDir(getenv)
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	if got != "/tmp/custom-columbus" {
		t.Errorf("DataDir = %q, want override", got)
	}
}

func TestDataDirFallsBackToOSConvention(t *testing.T) {
	getenv := func(k string) string {
		switch k {
		case "HOME":
			return "/home/tester"
		case "XDG_DATA_HOME":
			return "/home/tester/.xdg"
		}
		return ""
	}
	got, err := DataDir(getenv)
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	if !strings.Contains(got, "columbus") {
		t.Errorf("DataDir = %q, expected to contain columbus", got)
	}
}

func TestProjectPaths(t *testing.T) {
	pp := ProjectPaths("/data", "proj_abc")
	if pp.DBPath != filepath.Join("/data", "projects", "proj_abc", "columbus.sqlite") {
		t.Errorf("DBPath = %q", pp.DBPath)
	}
	if pp.LogPath != filepath.Join("/data", "projects", "proj_abc", "logs.jsonl") {
		t.Errorf("LogPath = %q", pp.LogPath)
	}
	if pp.ExportsDir != filepath.Join("/data", "projects", "proj_abc", "exports") {
		t.Errorf("ExportsDir = %q", pp.ExportsDir)
	}
}
