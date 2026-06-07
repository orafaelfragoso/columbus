// Package render projects a typed command result into one of three output
// formats: text (human), json (machine contract) and llm (markdown). All three
// are pure projections of the same typed Payload, so they can never silently
// diverge.
package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/orafaelfragoso/columbus/internal/contract"
)

// Format selects an output projection.
type Format int

const (
	FormatText Format = iota
	FormatJSON
	FormatLLM
)

// ParseFormat maps a flag string to a Format. Empty string is text (the
// default). Unknown values are a usage error.
func ParseFormat(s string) (Format, error) {
	switch s {
	case "", "text":
		return FormatText, nil
	case "json":
		return FormatJSON, nil
	case "llm":
		return FormatLLM, nil
	default:
		return FormatText, contract.Errorf(contract.CodeUsage, "unknown output format %q", s)
	}
}

// Options carries cross-cutting render settings.
type Options struct {
	Format Format
	// Color enables ANSI styling in text mode (set only for a TTY without
	// NO_COLOR/--no-color).
	Color bool
	// ContextLines is the padding around matched ranges for show/search.
	ContextLines int
}

// Payload is a typed command result that knows how to project itself to text
// and llm. The json projection is derived generically from the value's struct
// tags by Render, so payloads need not implement it.
type Payload interface {
	// CommandName is the value placed in the json envelope's "command" field.
	CommandName() string
	RenderText(w io.Writer, o Options) error
	RenderLLM(w io.Writer, o Options) error
}

// Render writes the payload to w in the requested format.
func Render(w io.Writer, p Payload, o Options) error {
	switch o.Format {
	case FormatJSON:
		return writeJSON(w, successEnvelope(p))
	case FormatLLM:
		return p.RenderLLM(w, o)
	default:
		return p.RenderText(w, o)
	}
}

// RenderError writes an error in the requested format. In json mode it emits
// the canonical error envelope; otherwise a plain human line.
func RenderError(w io.Writer, command string, e *contract.Error, o Options) error {
	if o.Format == FormatJSON {
		env := map[string]any{
			"ok":             false,
			"schema_version": contract.SchemaVersion,
			"command":        command,
			"error":          e,
		}
		return writeJSON(w, env)
	}
	if e.Hint != "" {
		_, err := fmt.Fprintf(w, "error [%s]: %s\n  hint: %s\n", e.Code, e.Message, e.Hint)
		return err
	}
	_, err := fmt.Fprintf(w, "error [%s]: %s\n", e.Code, e.Message)
	return err
}

// successEnvelope flattens a payload's json fields into a success envelope.
// Struct tags drive the payload fields; the envelope keys are merged on top.
func successEnvelope(p Payload) map[string]json.RawMessage {
	env := map[string]json.RawMessage{
		"ok":             mustRaw(true),
		"schema_version": mustRaw(contract.SchemaVersion),
		"command":        mustRaw(p.CommandName()),
	}
	raw, err := marshalNoEscape(p)
	if err != nil {
		// A payload that cannot marshal is a programming error; surface it
		// rather than emit a half envelope.
		env["error"] = mustRaw(&contract.Error{Code: contract.CodeStoreError, Message: err.Error()})
		env["ok"] = mustRaw(false)
		return env
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err == nil {
		for k, v := range fields {
			if _, reserved := env[k]; reserved {
				continue
			}
			env[k] = v
		}
	}
	return env
}

func writeJSON(w io.Writer, v any) error {
	raw, err := marshalNoEscape(v)
	if err != nil {
		return err
	}
	// For map payloads, marshal produces sorted keys (deterministic). Append a
	// single trailing newline.
	if _, err := w.Write(raw); err != nil {
		return err
	}
	_, err = w.Write([]byte("\n"))
	return err
}

// marshalNoEscape marshals v without HTML escaping, per the I/O contract.
func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	// Encoder appends a newline; trim it so callers control trailing bytes.
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func mustRaw(v any) json.RawMessage {
	raw, err := marshalNoEscape(v)
	if err != nil {
		panic(err)
	}
	return raw
}
