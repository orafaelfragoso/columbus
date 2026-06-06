package cli

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"testing"
)

// initProject sets up a git repo and a columbus project sharing one data dir.
func initProject(t *testing.T, work, data string) {
	t.Helper()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@e.com"}, {"config", "user.name", "T"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = work
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	var o, e bytes.Buffer
	if code := Execute([]string{"init"}, envForProject(t, work, data, &o, &e)); code != 0 {
		t.Fatalf("init exit = %d: %s", code, e.String())
	}
}

// runIn executes the CLI against a shared work/data project and returns
// stdout/stderr/exit.
func runProj(t *testing.T, work, data string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Execute(args, envForProject(t, work, data, &out, &errb))
	return out.String(), errb.String(), code
}

func TestEpicTaskLifecycleE2E(t *testing.T) {
	work, data := t.TempDir(), t.TempDir()
	initProject(t, work, data)

	out, errb, code := runProj(t, work, data, "epic", "add", "--title", "Ship search", "--json")
	if code != 0 {
		t.Fatalf("epic add exit = %d: %s", code, errb)
	}
	var epic struct {
		OK     bool   `json:"ok"`
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &epic); err != nil {
		t.Fatalf("not json: %v\n%s", err, out)
	}
	if !epic.OK || epic.ID != "epic_001" || epic.Status != "todo" {
		t.Fatalf("epic = %+v", epic)
	}

	_, errb, code = runProj(t, work, data, "task", "add", "--epic", "epic_001", "--title", "Index FTS", "--json")
	if code != 0 {
		t.Fatalf("task add exit = %d: %s", code, errb)
	}

	// Record a status change.
	out, _, code = runProj(t, work, data, "task", "status", "task_001", "--to", "in_progress", "--comment", "started", "--json")
	if code != 0 {
		t.Fatalf("task status exit = %d", code)
	}
	var task struct {
		Status string `json:"status"`
		Epic   string `json:"epic"`
	}
	json.Unmarshal([]byte(out), &task)
	if task.Status != "in_progress" || task.Epic != "epic_001" {
		t.Fatalf("task = %+v", task)
	}

	// List tasks filtered by status.
	out, _, _ = runProj(t, work, data, "task", "list", "--status", "in_progress", "--json")
	var list struct {
		Total int `json:"total"`
	}
	json.Unmarshal([]byte(out), &list)
	if list.Total != 1 {
		t.Fatalf("task list total = %d, want 1", list.Total)
	}
}

func TestEpicDeleteRequiresForceCLI(t *testing.T) {
	work, data := t.TempDir(), t.TempDir()
	initProject(t, work, data)
	runProj(t, work, data, "epic", "add", "--title", "x")

	_, _, code := runProj(t, work, data, "epic", "delete", "epic_001")
	if code != 2 {
		t.Fatalf("delete without --force exit = %d, want 2 (usage)", code)
	}
	_, _, code = runProj(t, work, data, "epic", "delete", "epic_001", "--force")
	if code != 0 {
		t.Fatalf("delete --force exit = %d, want 0", code)
	}
}

func TestEpicRefAndValidateCLI(t *testing.T) {
	work, data := t.TempDir(), t.TempDir()
	initProject(t, work, data)
	runProj(t, work, data, "epic", "add", "--title", "x")

	// A ref to a non-indexed file is stored but reported as drift.
	out, _, code := runProj(t, work, data, "epic", "ref", "epic_001", "--file", "ghost.go", "--json")
	if code != 0 {
		t.Fatalf("epic ref exit = %d", code)
	}
	if !contains(out, "does not resolve") {
		t.Fatalf("expected drift warning in %q", out)
	}

	out, _, code = runProj(t, work, data, "epic", "validate", "--json")
	if code != 0 {
		t.Fatalf("validate exit = %d", code)
	}
	var v struct {
		Unresolved int  `json:"unresolved"`
		Healthy    bool `json:"healthy"`
	}
	json.Unmarshal([]byte(out), &v)
	if v.Unresolved != 1 || v.Healthy {
		t.Fatalf("validate = %+v", v)
	}
}

func contains(s, sub string) bool { return bytes.Contains([]byte(s), []byte(sub)) }

func TestStatusRejectsUnknownCLI(t *testing.T) {
	work, data := t.TempDir(), t.TempDir()
	initProject(t, work, data)
	runProj(t, work, data, "epic", "add", "--title", "x")

	_, _, code := runProj(t, work, data, "epic", "status", "epic_001", "--to", "shipping")
	if code != 2 {
		t.Fatalf("unknown status exit = %d, want 2", code)
	}
}
