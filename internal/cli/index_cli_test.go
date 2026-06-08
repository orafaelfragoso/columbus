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

func TestReindexRequiresInstall(t *testing.T) {
	work := t.TempDir()
	var out, errb bytes.Buffer
	code := Execute([]string{"reindex", "--json"}, envForProject(t, work, t.TempDir(), &out, &errb))
	if code != 3 {
		t.Fatalf("exit = %d, want 3 (NOT_INITIALIZED)", code)
	}
}

func TestInstallThenReindexE2E(t *testing.T) {
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

	// install onboards and runs the first index in one step.
	var o1, e1 bytes.Buffer
	code := Execute([]string{"install", "--json"}, envForProject(t, work, data, &o1, &e1))
	if code != 0 {
		t.Fatalf("install exit = %d: %s", code, e1.String())
	}
	var inst struct {
		OK      bool `json:"ok"`
		Symbols int  `json:"symbols"`
		Files   int  `json:"files"`
	}
	if err := json.Unmarshal(o1.Bytes(), &inst); err != nil {
		t.Fatalf("install not JSON: %v\n%s", err, o1.String())
	}
	if !inst.OK || inst.Symbols < 1 || inst.Files < 1 {
		t.Errorf("bad install result: %+v", inst)
	}

	// reindex over an unchanged tree succeeds and reports status.
	var out, errb bytes.Buffer
	if code := Execute([]string{"reindex", "--json"}, envForProject(t, work, data, &out, &errb)); code != 0 {
		t.Fatalf("reindex exit = %d: %s", code, errb.String())
	}
	var env struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("reindex not JSON: %v\n%s", err, out.String())
	}
	if !env.OK {
		t.Errorf("bad reindex result: %+v", env)
	}
}
