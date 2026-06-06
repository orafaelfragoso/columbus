package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestDoctorRendersPayloadButExitsNonZero(t *testing.T) {
	work := t.TempDir()
	var out, errb bytes.Buffer
	code := Execute([]string{"doctor", "--json"}, envForProject(t, work, t.TempDir(), &out, &errb))
	if code != 3 {
		t.Fatalf("exit = %d, want 3", code)
	}
	// Even on failure, doctor emits its full report on stdout (not an error
	// envelope), so ok is true and checks are present.
	var env struct {
		OK      bool `json:"ok"`
		Healthy bool `json:"healthy"`
		Checks  []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if !env.OK {
		t.Error("doctor payload should be a success envelope (ok=true)")
	}
	if env.Healthy {
		t.Error("healthy should be false")
	}
	if len(env.Checks) == 0 {
		t.Error("expected checks in report")
	}
}
