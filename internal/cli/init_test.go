package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/orafaelfragoso/columbus/internal/clock"
	"github.com/orafaelfragoso/columbus/internal/ids"
)

// runIn executes the CLI with an isolated workdir and data dir.
func runIn(t *testing.T, workDir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	data := t.TempDir()
	var out, errb bytes.Buffer
	env := Env{
		Stdout:  &out,
		Stderr:  &errb,
		Clock:   clock.Fixed{T: time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)},
		IDs:     ids.Fixed{ID: "proj_e2e0000001"},
		WorkDir: workDir,
		Getenv: func(k string) string {
			if k == "COLUMBUS_DATA_DIR" {
				return data
			}
			return ""
		},
		Version: BuildInfo{Version: "test"},
	}
	code = Execute(args, env)
	return out.String(), errb.String(), code
}

func TestInitE2EJSON(t *testing.T) {
	work := t.TempDir()
	out, _, code := runIn(t, work, "init", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, out=%s", code, out)
	}
	var env struct {
		OK        bool   `json:"ok"`
		Command   string `json:"command"`
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if !env.OK || env.Command != "init" || env.ProjectID != "proj_e2e0000001" {
		t.Errorf("bad init envelope: %+v", env)
	}
	if _, err := os.Stat(filepath.Join(work, ".columbus.json")); err != nil {
		t.Errorf(".columbus.json not written: %v", err)
	}
}

func TestInitE2EText(t *testing.T) {
	work := t.TempDir()
	out, _, code := runIn(t, work, "init")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !bytes.Contains([]byte(out), []byte("proj_e2e0000001")) {
		t.Errorf("text output missing project id: %q", out)
	}
}
