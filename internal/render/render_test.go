package render

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/rafaelfragoso/columbus/internal/contract"
)

// samplePayload is a minimal Payload used to exercise the render layer.
type samplePayload struct {
	Greeting string   `json:"greeting"`
	Items    []string `json:"items"`
}

func (p samplePayload) CommandName() string { return "sample" }

func (p samplePayload) RenderText(w io.Writer, o Options) error {
	_, err := io.WriteString(w, p.Greeting+"\n")
	return err
}

func (p samplePayload) RenderLLM(w io.Writer, o Options) error {
	_, err := io.WriteString(w, "# "+p.Greeting+"\n")
	return err
}

func render(t *testing.T, p Payload, o Options) string {
	t.Helper()
	var buf bytes.Buffer
	if err := Render(&buf, p, o); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return buf.String()
}

func TestJSONEnvelopeWrapsPayloadAtTopLevel(t *testing.T) {
	out := render(t, samplePayload{Greeting: "hi", Items: []string{"a"}}, Options{Format: FormatJSON})

	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if env["ok"] != true {
		t.Errorf("ok = %v, want true", env["ok"])
	}
	if env["schema_version"] != float64(contract.SchemaVersion) {
		t.Errorf("schema_version = %v, want %d", env["schema_version"], contract.SchemaVersion)
	}
	if env["command"] != "sample" {
		t.Errorf("command = %v, want sample", env["command"])
	}
	if env["greeting"] != "hi" {
		t.Errorf("payload field greeting not flattened: %v", env["greeting"])
	}
}

func TestJSONEndsWithSingleNewline(t *testing.T) {
	out := render(t, samplePayload{Greeting: "hi"}, Options{Format: FormatJSON})
	if !strings.HasSuffix(out, "}\n") {
		t.Errorf("json should end with newline: %q", out)
	}
	if strings.Count(out, "\n") != 1 {
		t.Errorf("json should be single-line (one trailing newline): %q", out)
	}
}

func TestJSONDoesNotEscapeHTML(t *testing.T) {
	out := render(t, samplePayload{Greeting: "a < b && c > d"}, Options{Format: FormatJSON})
	if !strings.Contains(out, "a < b && c > d") {
		t.Errorf("HTML chars should not be escaped: %q", out)
	}
}

func TestTextProjection(t *testing.T) {
	out := render(t, samplePayload{Greeting: "hello"}, Options{Format: FormatText})
	if out != "hello\n" {
		t.Errorf("text = %q, want %q", out, "hello\n")
	}
}

func TestLLMProjection(t *testing.T) {
	out := render(t, samplePayload{Greeting: "hello"}, Options{Format: FormatLLM})
	if out != "# hello\n" {
		t.Errorf("llm = %q, want %q", out, "# hello\n")
	}
}

func TestRenderErrorJSONEnvelope(t *testing.T) {
	var buf bytes.Buffer
	e := &contract.Error{Code: contract.CodeIndexMissing, Message: "no index", Hint: "run columbus index"}
	if err := RenderError(&buf, "search", e, Options{Format: FormatJSON}); err != nil {
		t.Fatalf("RenderError: %v", err)
	}
	var env struct {
		OK            bool   `json:"ok"`
		SchemaVersion int    `json:"schema_version"`
		Command       string `json:"command"`
		Error         struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Hint    string `json:"hint"`
		} `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, buf.String())
	}
	if env.OK {
		t.Error("ok should be false")
	}
	if env.Command != "search" {
		t.Errorf("command = %q", env.Command)
	}
	if env.Error.Code != "INDEX_MISSING" || env.Error.Message != "no index" || env.Error.Hint != "run columbus index" {
		t.Errorf("error payload wrong: %+v", env.Error)
	}
}

func TestRenderErrorTextGoesPlain(t *testing.T) {
	var buf bytes.Buffer
	e := &contract.Error{Code: contract.CodeNotFound, Message: "missing", Hint: "check the path"}
	if err := RenderError(&buf, "show", e, Options{Format: FormatText}); err != nil {
		t.Fatalf("RenderError: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "missing") {
		t.Errorf("text error should contain message: %q", got)
	}
	if !strings.Contains(got, "check the path") {
		t.Errorf("text error should contain hint: %q", got)
	}
	if !strings.Contains(got, "NOT_FOUND") {
		t.Errorf("text error should contain code: %q", got)
	}
}

func TestParseFormat(t *testing.T) {
	cases := map[string]Format{"": FormatText, "text": FormatText, "json": FormatJSON, "llm": FormatLLM}
	for in, want := range cases {
		got, err := ParseFormat(in)
		if err != nil {
			t.Errorf("ParseFormat(%q) error: %v", in, err)
		}
		if got != want {
			t.Errorf("ParseFormat(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := ParseFormat("yaml"); err == nil {
		t.Error("ParseFormat(yaml) should error")
	}
}
