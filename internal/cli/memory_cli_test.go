package cli

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"testing"
)

// initProject sets up a git repo and onboards a columbus project (install runs
// the first index) sharing one data dir.
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
	if code := Execute([]string{"install"}, envForProject(t, work, data, &o, &e)); code != 0 {
		t.Fatalf("install exit = %d: %s", code, e.String())
	}
}

// runProj executes the CLI against a shared work/data project and returns
// stdout/stderr/exit.
func runProj(t *testing.T, work, data string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Execute(args, envForProject(t, work, data, &out, &errb))
	return out.String(), errb.String(), code
}

func contains(s, sub string) bool { return bytes.Contains([]byte(s), []byte(sub)) }

func TestMemoryLifecycleE2E(t *testing.T) {
	work, data := t.TempDir(), t.TempDir()
	initProject(t, work, data)

	out, errb, code := runProj(t, work, data, "memory", "add", "adr",
		"--title", "Use SQLite", "--body", "single-file local store", "--tag", "storage", "--json")
	if code != 0 {
		t.Fatalf("memory add adr exit = %d: %s", code, errb)
	}
	var rec struct {
		OK   bool   `json:"ok"`
		ID   string `json:"id"`
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal([]byte(out), &rec); err != nil {
		t.Fatalf("not json: %v\n%s", err, out)
	}
	if !rec.OK || rec.ID != "mem_001" || rec.Kind != "adr" {
		t.Fatalf("adr = %+v", rec)
	}

	for _, kind := range []string{"plan", "documentation"} {
		if _, errb, code := runProj(t, work, data, "memory", "add", kind, "--title", "t "+kind, "--json"); code != 0 {
			t.Fatalf("memory add %s exit = %d: %s", kind, code, errb)
		}
	}

	// Old work/context kinds are gone.
	out, _, code = runProj(t, work, data, "memory", "add", "decision", "--title", "x", "--json")
	if code != 2 {
		t.Fatalf("memory add decision exit = %d, want 2 (INVALID_KIND)", code)
	}
	if !contains(out, "INVALID_KIND") {
		t.Fatalf("expected INVALID_KIND in %s", out)
	}

	// Update + list + remove round-trip.
	if _, errb, code := runProj(t, work, data, "memory", "update", "mem_001", "--title", "Use SQLite (WAL)", "--add-tag", "db", "--json"); code != 0 {
		t.Fatalf("memory update exit = %d: %s", code, errb)
	}
	out, _, code = runProj(t, work, data, "memory", "list", "--kind", "adr", "--json")
	if code != 0 {
		t.Fatalf("memory list exit = %d", code)
	}
	var list struct {
		Total    int `json:"total"`
		Memories []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"memories"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		t.Fatalf("not json: %v\n%s", err, out)
	}
	if list.Total != 1 || list.Memories[0].Title != "Use SQLite (WAL)" {
		t.Fatalf("list = %+v", list)
	}
	if _, errb, code := runProj(t, work, data, "memory", "remove", "mem_001", "--json"); code != 0 {
		t.Fatalf("memory remove exit = %d: %s", code, errb)
	}
}

func TestSearchRejectsWorkKinds(t *testing.T) {
	work, data := t.TempDir(), t.TempDir()
	initProject(t, work, data)
	_, _, code := runProj(t, work, data, "search", "anything", "--kind", "epic")
	if code != 2 {
		t.Fatalf("search --kind epic exit = %d, want 2 (usage)", code)
	}
}

func TestShowDropsWorkSubcommands(t *testing.T) {
	work, data := t.TempDir(), t.TempDir()
	initProject(t, work, data)
	out, _, _ := runProj(t, work, data, "show", "--help")
	if contains(out, "epic") || contains(out, "task") {
		t.Fatalf("show help still lists work subcommands:\n%s", out)
	}
	if !contains(out, "memory") {
		t.Fatalf("show help missing memory subcommand:\n%s", out)
	}
	// Unknown subcommands are usage errors, not silent help.
	if _, _, code := runProj(t, work, data, "show", "epic", "epic_001"); code != 2 {
		t.Fatalf("show epic exit = %d, want 2 (usage)", code)
	}
	if _, _, code := runProj(t, work, data, "memory", "bogus"); code != 2 {
		t.Fatalf("memory bogus exit = %d, want 2 (usage)", code)
	}
}
