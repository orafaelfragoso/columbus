package search

import "testing"

func TestNameMatchTiers(t *testing.T) {
	tok := tokenize("parse")
	if got := nameMatch(tok, "parse"); got != 1.0 {
		t.Errorf("exact = %v, want 1.0", got)
	}
	if got := nameMatch(tok, "parseConfig"); got != 0.7 {
		t.Errorf("prefix = %v, want 0.7", got)
	}
	if got := nameMatch(tok, "reparseAll"); got != 0.4 {
		t.Errorf("substring = %v, want 0.4", got)
	}
	if got := nameMatch(tok, "unrelated"); got != 0 {
		t.Errorf("none = %v, want 0", got)
	}
}

func TestScoreInUnitRange(t *testing.T) {
	tok := tokenize("server")
	s := signals{name: "Server", signature: "type Server struct", path: "internal/server.go",
		role: "impl", importedByCount: 10, hasTests: true, hasMemory: true}
	got := score(tok, s)
	if got < 0 || got > 1 {
		t.Errorf("score %v out of [0,1]", got)
	}
	if got <= 0 {
		t.Errorf("strong match should score > 0, got %v", got)
	}
}

func TestExactNameBeatsSubstring(t *testing.T) {
	tok := tokenize("user")
	exact := score(tok, signals{name: "user", role: "impl"})
	sub := score(tok, signals{name: "currentUserName", role: "impl"})
	if exact <= sub {
		t.Errorf("exact (%v) should beat substring (%v)", exact, sub)
	}
}

func TestImplBeatsTestAllElseEqual(t *testing.T) {
	tok := tokenize("handler")
	impl := score(tok, signals{name: "handler", role: "impl"})
	test := score(tok, signals{name: "handler", role: "test"})
	if impl <= test {
		t.Errorf("impl (%v) should outrank test (%v)", impl, test)
	}
}

func TestWhyTemplates(t *testing.T) {
	tok := tokenize("parse")
	if w := why(tok, signals{name: "parse", role: "impl"}); w != "exact name match" {
		t.Errorf("why = %q, want exact name match", w)
	}
	if w := why(tok, signals{name: "parseConfig", role: "impl"}); w != "name prefix match" {
		t.Errorf("why = %q, want name prefix match", w)
	}
}

func TestRiskLevels(t *testing.T) {
	if r := riskLevel(signals{importedByCount: 9}); r != "medium" {
		t.Errorf("central risk = %q, want medium", r)
	}
	if r := riskLevel(signals{}); r != "low" {
		t.Errorf("default risk = %q, want low", r)
	}
}

func TestTokenize(t *testing.T) {
	got := tokenize("parseConfig, foo-bar_baz")
	want := []string{"parseconfig", "foo", "bar_baz"}
	if len(got) != len(want) {
		t.Fatalf("tokens = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestBuildFTSMatch(t *testing.T) {
	if got := buildFTSMatch([]string{"foo", "bar"}); got != `"foo"* OR "bar"*` {
		t.Errorf("match = %q", got)
	}
}
