// Package logging provides a per-project JSONL logger (slog + size-capped
// rotation) distinct from per-invocation stderr diagnostics. The timestamp
// comes from the injected clock so logs are deterministic in tests.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	lumberjack "gopkg.in/natefinch/lumberjack.v2"

	"github.com/rafaelfragoso/columbus/internal/clock"
)

// ParseLevel maps COLUMBUS_LOG_LEVEL to a slog level (default info).
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// New creates a project logger writing JSONL to path with size-capped rotation.
// It returns the logger and a closer for the underlying file. Every record
// carries project_id and a clock-derived timestamp.
func New(path string, level slog.Level, clk clock.Clock, projectID string) (*slog.Logger, io.Closer, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, err
	}
	rotator := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    10, // megabytes
		MaxBackups: 2,
		Compress:   false,
	}
	base := slog.NewJSONHandler(rotator, &slog.HandlerOptions{Level: level})
	handler := &clockHandler{inner: base, clk: clk}
	logger := slog.New(handler).With(slog.String("project_id", projectID))
	return logger, rotator, nil
}

// NewDiscard returns a logger that drops everything (used when no project log is
// available, e.g. uninitialized projects).
func NewDiscard() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

// clockHandler overrides each record's timestamp with the injected clock.
type clockHandler struct {
	inner slog.Handler
	clk   clock.Clock
}

func (h *clockHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h *clockHandler) Handle(ctx context.Context, r slog.Record) error {
	r.Time = h.clk.Now()
	return h.inner.Handle(ctx, r)
}

func (h *clockHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &clockHandler{inner: h.inner.WithAttrs(attrs), clk: h.clk}
}

func (h *clockHandler) WithGroup(name string) slog.Handler {
	return &clockHandler{inner: h.inner.WithGroup(name), clk: h.clk}
}
