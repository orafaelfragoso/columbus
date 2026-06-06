package contract

import "testing"

func TestCodeExitMapping(t *testing.T) {
	cases := map[Code]ExitCode{
		CodeUsage:             ExitUsage,
		CodeConfigInvalid:     ExitUsage,
		CodeInvalidKind:       ExitUsage,
		CodeNotInitialized:    ExitNotInitialized,
		CodeIndexMissing:      ExitNotInitialized,
		CodeIndexLocked:       ExitRuntime,
		CodeSchemaTooNew:      ExitRuntime,
		CodeNotFound:          ExitRuntime,
		CodeStoreError:        ExitRuntime,
		CodeDependencyMissing: ExitRuntime,
	}
	for code, want := range cases {
		if got := code.Exit(); got != want {
			t.Errorf("%s.Exit() = %d, want %d", code, got, want)
		}
	}
}

func TestUnknownCodeExitsRuntime(t *testing.T) {
	if got := Code("MYSTERY").Exit(); got != ExitRuntime {
		t.Errorf("unknown code exit = %d, want %d", got, ExitRuntime)
	}
}

func TestErrorImplementsError(t *testing.T) {
	var err error = &Error{Code: CodeNotFound, Message: "nope", Hint: "try again"}
	if err.Error() != "nope" {
		t.Errorf("Error() = %q, want %q", err.Error(), "nope")
	}
}

func TestNewErrorf(t *testing.T) {
	e := Errorf(CodeUsage, "bad flag %q", "--frob")
	if e.Code != CodeUsage {
		t.Errorf("code = %s, want %s", e.Code, CodeUsage)
	}
	if e.Message != `bad flag "--frob"` {
		t.Errorf("message = %q", e.Message)
	}
}

func TestAsErrorWrapsUnknown(t *testing.T) {
	plain := &Error{Code: CodeStoreError, Message: "boom"}
	got := AsError(plain)
	if got != plain {
		t.Errorf("AsError should pass through *Error unchanged")
	}
}
