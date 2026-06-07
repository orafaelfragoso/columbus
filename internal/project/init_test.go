package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orafaelfragoso/columbus/internal/config"
	"github.com/orafaelfragoso/columbus/internal/ids"
)

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@e.com"}, {"config", "user.name", "T"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

// initParams builds InitParams pointing at a fresh workdir and isolated data dir.
func initParams(t *testing.T, projectID string) InitParams {
	t.Helper()
	work := t.TempDir()
	data := t.TempDir()
	getenv := func(k string) string {
		if k == "COLUMBUS_DATA_DIR" {
			return data
		}
		return ""
	}
	return InitParams{WorkDir: work, IDs: ids.Fixed{ID: projectID}, Getenv: getenv}
}

func TestInitCreatesConfigWithProjectID(t *testing.T) {
	p := initParams(t, "proj_aaaa1111")
	res, err := Init(p)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if res.ProjectID != "proj_aaaa1111" {
		t.Errorf("project_id = %q", res.ProjectID)
	}
	loaded, err := config.Load(filepath.Join(p.WorkDir, config.FileName))
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if loaded.Config.ProjectID != "proj_aaaa1111" {
		t.Errorf("persisted project_id = %q", loaded.Config.ProjectID)
	}
}

func TestInitCreatesDatabase(t *testing.T) {
	p := initParams(t, "proj_bbbb2222")
	res, err := Init(p)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := os.Stat(res.DBPath); err != nil {
		t.Errorf("db not created at %s: %v", res.DBPath, err)
	}
}

func TestInitAddsGitExclude(t *testing.T) {
	p := initParams(t, "proj_cccc3333")
	initGitRepo(t, p.WorkDir)
	res, err := Init(p)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !res.GitExcluded {
		t.Error("expected GitExcluded true in a git repo")
	}
	data, err := os.ReadFile(filepath.Join(p.WorkDir, ".git", "info", "exclude"))
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	if !strings.Contains(string(data), config.FileName) {
		t.Error("exclude missing .columbus.json")
	}
}

func TestInitOutsideGitRepoStillWorks(t *testing.T) {
	p := initParams(t, "proj_dddd4444")
	res, err := Init(p)
	if err != nil {
		t.Fatalf("Init outside git: %v", err)
	}
	if res.GitExcluded {
		t.Error("GitExcluded should be false outside a repo")
	}
}

func TestInitIsIdempotentAndKeepsProjectID(t *testing.T) {
	p := initParams(t, "proj_eeee5555")
	first, err := Init(p)
	if err != nil {
		t.Fatalf("first Init: %v", err)
	}
	// Second init with a DIFFERENT id source must not regenerate.
	p.IDs = ids.Fixed{ID: "proj_ffff6666"}
	second, err := Init(p)
	if err != nil {
		t.Fatalf("second Init: %v", err)
	}
	if !second.AlreadyInitialized {
		t.Error("second Init should report AlreadyInitialized")
	}
	if second.ProjectID != first.ProjectID {
		t.Errorf("project_id changed on re-init: %q -> %q", first.ProjectID, second.ProjectID)
	}
}
