package ids

import (
	"regexp"
	"testing"
)

func TestCryptoSourceProjectIDFormat(t *testing.T) {
	s := Crypto{}
	re := regexp.MustCompile(`^proj_[0-9a-f]{16}$`)
	id := s.ProjectID()
	if !re.MatchString(id) {
		t.Errorf("ProjectID() = %q, want proj_ + 16 hex", id)
	}
}

func TestCryptoSourceProjectIDsAreDistinct(t *testing.T) {
	s := Crypto{}
	seen := map[string]bool{}
	for range 100 {
		id := s.ProjectID()
		if seen[id] {
			t.Fatalf("duplicate project id %q", id)
		}
		seen[id] = true
	}
}

func TestFixedSourceIsDeterministic(t *testing.T) {
	s := Fixed{ID: "proj_deadbeefdeadbeef"}
	if got := s.ProjectID(); got != "proj_deadbeefdeadbeef" {
		t.Errorf("ProjectID() = %q", got)
	}
}
