package logging

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rafaelfragoso/columbus/internal/clock"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"":      slog.LevelInfo,
		"info":  slog.LevelInfo,
		"DEBUG": slog.LevelDebug,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}
	for in, want := range cases {
		if got := ParseLevel(in); got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestLoggerWritesJSONLWithInjectedClockAndProjectID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs.jsonl")
	fixed := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	logger, closer, err := New(path, slog.LevelInfo, clock.Fixed{T: fixed}, "proj_abc")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	logger.Info("index", slog.Int("indexed", 5))
	closer.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	var entry map[string]any
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("log line not JSON: %v\n%s", err, data)
	}
	if entry["msg"] != "index" {
		t.Errorf("msg = %v", entry["msg"])
	}
	if entry["project_id"] != "proj_abc" {
		t.Errorf("project_id = %v", entry["project_id"])
	}
	if entry["indexed"] != float64(5) {
		t.Errorf("indexed = %v", entry["indexed"])
	}
	if ts, _ := entry["time"].(string); ts == "" || ts[:4] != "2026" {
		t.Errorf("time = %v, want injected 2026 timestamp", entry["time"])
	}
}

func TestInfoLevelDropsDebug(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs.jsonl")
	logger, closer, _ := New(path, slog.LevelInfo, clock.Fixed{T: time.Now()}, "p")
	logger.Debug("read", slog.String("cmd", "search"))
	closer.Close()
	data, _ := os.ReadFile(path)
	if len(data) != 0 {
		t.Errorf("debug should be dropped at info level, got: %s", data)
	}
}
