package doctor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rafaelfragoso/columbus/internal/config"
)

func paramsFor(t *testing.T, work string) Params {
	t.Helper()
	data := t.TempDir()
	return Params{
		WorkDir: work,
		Getenv: func(k string) string {
			if k == "COLUMBUS_DATA_DIR" {
				return data
			}
			return ""
		},
		Version: "test",
	}
}

func findCheck(checks []Check, name string) (Check, bool) {
	for _, c := range checks {
		if c.Name == name {
			return c, true
		}
	}
	return Check{}, false
}

func TestDoctorReportsGitAndGrammars(t *testing.T) {
	res, _ := Run(paramsFor(t, t.TempDir()))
	if c, ok := findCheck(res.Checks, "git"); !ok || c.Status != StatusOK {
		t.Errorf("git check = %+v", c)
	}
	if c, ok := findCheck(res.Checks, "grammars"); !ok || c.Status != StatusOK {
		t.Errorf("grammars check = %+v", c)
	}
}

func TestDoctorNotInitialized(t *testing.T) {
	res, code := Run(paramsFor(t, t.TempDir()))
	if code != "NOT_INITIALIZED" {
		t.Errorf("code = %q, want NOT_INITIALIZED", code)
	}
	if res.Healthy {
		t.Error("should not be healthy without a project")
	}
	if c, _ := findCheck(res.Checks, "config"); c.Status != StatusFail {
		t.Errorf("config check = %+v, want fail", c)
	}
	if c, _ := findCheck(res.Checks, "database"); c.Status != StatusSkip {
		t.Errorf("database check = %+v, want skip", c)
	}
}

func TestDoctorInitializedButNoIndex(t *testing.T) {
	work := t.TempDir()
	p := paramsFor(t, work)
	// Write a valid config and create the empty DB so config passes.
	cfg := config.Default()
	cfg.ProjectID = "proj_doctor0001"
	if err := config.Save(filepath.Join(work, config.FileName), cfg); err != nil {
		t.Fatal(err)
	}

	res, code := Run(p)
	if c, _ := findCheck(res.Checks, "config"); c.Status != StatusOK {
		t.Errorf("config check = %+v, want ok", c)
	}
	// No DB created yet -> database warns (not fail) and stays healthy/exit 0.
	if c, _ := findCheck(res.Checks, "database"); c.Status != StatusWarn {
		t.Errorf("database check = %+v, want warn", c)
	}
	if code != "" {
		t.Errorf("warnings should keep exit code empty, got %q", code)
	}
	if !res.Healthy {
		t.Error("warnings should still be healthy")
	}
}

func TestDoctorInvalidConfig(t *testing.T) {
	work := t.TempDir()
	if err := os.WriteFile(filepath.Join(work, config.FileName), []byte(`{ "schema_version": 1 }`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, code := Run(paramsFor(t, work))
	if code != "CONFIG_INVALID" {
		t.Errorf("code = %q, want CONFIG_INVALID", code)
	}
}
