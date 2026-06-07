package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/orafaelfragoso/columbus/internal/clock"
	"github.com/orafaelfragoso/columbus/internal/ids"
)

// envForProject builds an Env sharing one workdir and data dir across calls.
func envForProject(t *testing.T, work, data string, out, errb *bytes.Buffer) Env {
	t.Helper()
	return Env{
		Stdout:  out,
		Stderr:  errb,
		Clock:   clock.Fixed{T: time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)},
		IDs:     ids.Fixed{ID: "proj_cli0000001"},
		WorkDir: work,
		Getenv: func(k string) string {
			if k == "COLUMBUS_DATA_DIR" {
				return data
			}
			return ""
		},
		Version: BuildInfo{Version: "test"},
	}
}

func TestIndexRequiresInit(t *testing.T) {
	work := t.TempDir()
	var out, errb bytes.Buffer
	code := Execute([]string{"index", "--json"}, envForProject(t, work, t.TempDir(), &out, &errb))
	if code != 3 {
		t.Fatalf("exit = %d, want 3 (NOT_INITIALIZED)", code)
	}
}

func TestInitThenIndexE2E(t *testing.T) {
	work := t.TempDir()
	data := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@e.com"}, {"config", "user.name", "T"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = work
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v\n%s", err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(work, "svc.go"), []byte("package svc\n\nfunc New() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var o1, e1 bytes.Buffer
	if code := Execute([]string{"init"}, envForProject(t, work, data, &o1, &e1)); code != 0 {
		t.Fatalf("init exit = %d: %s", code, e1.String())
	}

	var out, errb bytes.Buffer
	code := Execute([]string{"index", "--json"}, envForProject(t, work, data, &out, &errb))
	if code != 0 {
		t.Fatalf("index exit = %d: %s", code, errb.String())
	}
	var env struct {
		OK      bool `json:"ok"`
		Indexed int  `json:"indexed"`
		Symbols int  `json:"symbols"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if !env.OK || env.Indexed < 1 || env.Symbols < 1 {
		t.Errorf("bad index result: %+v", env)
	}
}
