package gitrepo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestDiscoverInsideRepo(t *testing.T) {
	dir := initRepo(t)
	info, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !info.IsRepo {
		t.Fatal("expected IsRepo true")
	}
	if info.GitDir == "" {
		t.Error("GitDir should be set")
	}
}

func TestDiscoverOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	info, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover should not error outside repo: %v", err)
	}
	if info.IsRepo {
		t.Error("expected IsRepo false outside a repo")
	}
}

func TestAddExcludeIsIdempotent(t *testing.T) {
	dir := initRepo(t)
	info, _ := Discover(dir)

	for range 2 {
		if err := info.AddExclude(".columbus.json"); err != nil {
			t.Fatalf("AddExclude: %v", err)
		}
	}
	data, err := os.ReadFile(filepath.Join(info.GitDir, "info", "exclude"))
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	if n := strings.Count(string(data), ".columbus.json"); n != 1 {
		t.Errorf("exclude entry count = %d, want 1 (idempotent)", n)
	}
}

func TestAddExcludeNoOpOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	info, _ := Discover(dir)
	if err := info.AddExclude(".columbus.json"); err != nil {
		t.Errorf("AddExclude outside repo should be a no-op, got %v", err)
	}
}
