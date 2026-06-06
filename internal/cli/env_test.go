package cli

import (
	"errors"
	"testing"

	"github.com/rafaelfragoso/columbus/internal/contract"
)

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
