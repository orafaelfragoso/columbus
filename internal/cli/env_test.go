package cli

import (
	"bytes"
	"errors"
	"testing"

	"github.com/rafaelfragoso/columbus/internal/contract"
)

// resolveColor runs resolveRenderOptions with a non-TTY stdout and the given
// env, returning whether color was enabled. A bytes.Buffer is never a TTY, so
// any true result must come from an explicit override (e.g. FORCE_COLOR).
func resolveColor(t *testing.T, env map[string]string, asJSON, asLLM, noColor bool) bool {
	t.Helper()
	e := &Env{Stdout: &bytes.Buffer{}, Getenv: func(k string) string { return env[k] }}
	if err := e.resolveRenderOptions(asJSON, asLLM, noColor); err != nil {
		t.Fatalf("resolveRenderOptions: %v", err)
	}
	return e.renderOpts.Color
}

func TestColorDetectionPrecedence(t *testing.T) {
	cases := []struct {
		name  string
		env   map[string]string
		json  bool
		llm   bool
		noClr bool
		want  bool
	}{
		{"FORCE_COLOR forces color without a TTY", map[string]string{"FORCE_COLOR": "1"}, false, false, false, true},
		{"NO_COLOR beats FORCE_COLOR", map[string]string{"NO_COLOR": "1", "FORCE_COLOR": "1"}, false, false, false, false},
		{"--no-color beats FORCE_COLOR", map[string]string{"FORCE_COLOR": "1"}, false, false, true, false},
		{"FORCE_COLOR beats TERM=dumb", map[string]string{"FORCE_COLOR": "1", "TERM": "dumb"}, false, false, false, true},
		{"FORCE_COLOR beats CI", map[string]string{"FORCE_COLOR": "1", "CI": "true"}, false, false, false, true},
		{"json never colors even with FORCE_COLOR", map[string]string{"FORCE_COLOR": "1"}, true, false, false, false},
		{"llm never colors even with FORCE_COLOR", map[string]string{"FORCE_COLOR": "1"}, false, true, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveColor(t, c.env, c.json, c.llm, c.noClr); got != c.want {
				t.Errorf("color = %v, want %v", got, c.want)
			}
		})
	}
}

func TestResolveWorkDirReturnsDirectory(t *testing.T) {
	wd, err := ResolveWorkDir(func() (string, error) { return "/work/here", nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wd != "/work/here" {
		t.Errorf("WorkDir = %q, want /work/here", wd)
	}
}

func TestResolveWorkDirSurfacesGetwdFailure(t *testing.T) {
	wd, err := ResolveWorkDir(func() (string, error) {
		return "", errors.New("getcwd: no such file or directory")
	})
	if wd != "" {
		t.Errorf("WorkDir = %q on failure, want empty", wd)
	}
	var ce *contract.Error
	if !errors.As(err, &ce) {
		t.Fatalf("err = %T, want *contract.Error", err)
	}
	if ce.Message == "" {
		t.Error("error message should explain the working-directory failure")
	}
}
