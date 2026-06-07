package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/orafaelfragoso/columbus/internal/clock"
	"github.com/orafaelfragoso/columbus/internal/ids"
)

// run executes the CLI in-process with the given args and returns stdout,
// stderr and the exit code.
func run(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	env := Env{
		Stdout:  &out,
		Stderr:  &errb,
		Clock:   clock.Fixed{T: time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)},
		IDs:     ids.Fixed{ID: "proj_deadbeefdeadbeef"},
		Getenv:  func(string) string { return "" },
		Version: BuildInfo{Version: "1.2.3", Commit: "abc123", Date: "2026-06-06"},
	}
	code = Execute(args, env)
	return out.String(), errb.String(), code
}

func TestVersionTextProjection(t *testing.T) {
	out, _, code := run(t, "version")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, "1.2.3") {
		t.Errorf("stdout missing version: %q", out)
	}
}

func TestVersionFlagMatchesSubcommand(t *testing.T) {
	flagOut, _, _ := run(t, "--version")
	cmdOut, _, _ := run(t, "version")
	if flagOut != cmdOut {
		t.Errorf("--version (%q) != version (%q)", flagOut, cmdOut)
	}
}

func TestVersionJSONEnvelope(t *testing.T) {
	out, _, code := run(t, "version", "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var env struct {
		OK            bool   `json:"ok"`
		SchemaVersion int    `json:"schema_version"`
		Command       string `json:"command"`
		Version       string `json:"version"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, out)
	}
	if !env.OK || env.Command != "version" || env.Version != "1.2.3" {
		t.Errorf("bad envelope: %+v", env)
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	_, _, code := run(t, "frobnicate")
	if code != 2 {
		t.Errorf("unknown command exit = %d, want 2", code)
	}
}

func TestJSONErrorGoesToStdoutNotStderr(t *testing.T) {
	out, errb, code := run(t, "_selftest", "--fail", "INDEX_MISSING", "--json")
	if code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
	if strings.TrimSpace(errb) != "" {
		t.Errorf("stderr should be empty in json mode, got %q", errb)
	}
	var env struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, out)
	}
	if env.OK || env.Error.Code != "INDEX_MISSING" {
		t.Errorf("bad error envelope: %+v", env)
	}
}

func TestTextErrorGoesToStderr(t *testing.T) {
	out, errb, code := run(t, "_selftest", "--fail", "NOT_FOUND")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("stdout should be empty on text error, got %q", out)
	}
	if !strings.Contains(errb, "NOT_FOUND") {
		t.Errorf("stderr missing error: %q", errb)
	}
}

func TestSelftestSuccessPayload(t *testing.T) {
	out, _, code := run(t, "_selftest", "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, "\"command\":\"_selftest\"") && !strings.Contains(out, "\"command\": \"_selftest\"") {
		t.Errorf("missing command in payload: %q", out)
	}
}

func TestRejectsBothJSONAndLLM(t *testing.T) {
	_, _, code := run(t, "version", "--json", "--llm")
	if code != 2 {
		t.Errorf("exit = %d, want 2 (usage)", code)
	}
}
