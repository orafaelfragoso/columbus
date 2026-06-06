package extract

import "testing"

func TestRegistryCompilesAllQueries(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	for _, ext := range []string{".go", ".ts", ".tsx", ".js", ".py", ".md"} {
		if _, ok := r.ForPath("file" + ext); !ok {
			t.Errorf("no extractor for %s", ext)
		}
	}
}
