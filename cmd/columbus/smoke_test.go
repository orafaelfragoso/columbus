package main

import (
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildBinary compiles the columbus binary once into a temp dir and returns its
// path. It builds with the fts5 tag so the smoke set also catches missing
// build-tag wiring.
func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "columbus")
	cmd := exec.Command("go", "build", "-tags", "fts5", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

func TestBinaryVersionJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary smoke in -short")
	}
	bin := buildBinary(t)
	out, err := exec.Command(bin, "version", "--json").Output()
	if err != nil {
		t.Fatalf("run --version: %v", err)
	}
	var env struct {
		OK      bool   `json:"ok"`
		Command string `json:"command"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, out)
	}
	if !env.OK || env.Command != "version" {
		t.Errorf("bad envelope: %s", out)
	}
}

func TestBinaryUnknownCommandExit2(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary smoke in -short")
	}
	bin := buildBinary(t)
	err := exec.Command(bin, "definitely-not-a-command").Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected exit error, got %v", err)
	}
	if ee.ExitCode() != 2 {
		t.Errorf("exit = %d, want 2", ee.ExitCode())
	}
}

func TestBinaryHelpMentionsColumbus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary smoke in -short")
	}
	bin := buildBinary(t)
	out, _ := exec.Command(bin, "--help").CombinedOutput()
	if !strings.Contains(string(out), "columbus") {
		t.Errorf("help missing columbus: %s", out)
	}
}
