// Package contract defines the stable machine-facing surface of Columbus:
// the JSON schema version, the canonical error codes, and their mapping to
// process exit codes. The plugin consumes these as an API contract.
package contract

import (
	"errors"
	"fmt"
)

// SchemaVersion is the version of the --json output contract. Bump on any
// breaking change to the machine-facing payload shape.
const SchemaVersion = 1

// ExitCode is a process exit status. The plugin branches on these.
type ExitCode int

const (
	// ExitOK indicates success (including "usable with warnings").
	ExitOK ExitCode = 0
	// ExitRuntime indicates a runtime error.
	ExitRuntime ExitCode = 1
	// ExitUsage indicates a usage error (bad flags/args/config).
	ExitUsage ExitCode = 2
	// ExitNotInitialized indicates a recoverable "not set up" state the
	// plugin branches on (no config, or index missing).
	ExitNotInitialized ExitCode = 3
	// ExitTransient indicates a temporary, retryable failure (e.g. the index
	// writer lock is held). Distinct from ExitRuntime so a caller's retry
	// logic can branch on the exit code alone, without parsing the envelope.
	ExitTransient ExitCode = 4
)

// Code is a canonical, stable error identifier carried in the JSON error
// envelope.
type Code string

const (
	CodeUsage             Code = "USAGE"
	CodeConfigInvalid     Code = "CONFIG_INVALID"
	CodeInvalidKind       Code = "INVALID_KIND"
	CodeNotInitialized    Code = "NOT_INITIALIZED"
	CodeIndexMissing      Code = "INDEX_MISSING"
	CodeIndexLocked       Code = "INDEX_LOCKED"
	CodeSchemaTooNew      Code = "SCHEMA_TOO_NEW"
	CodeNotFound          Code = "NOT_FOUND"
	CodeStoreError        Code = "STORE_ERROR"
	CodeDependencyMissing Code = "DEPENDENCY_MISSING"
	// CodeRuntimeMissing means a required runtime could not be loaded. Kept for
	// compatibility with older clients and error envelopes.
	CodeRuntimeMissing Code = "RUNTIME_MISSING"
	// CodeEmbedFailure means an embedding session run or text encode failed.
	CodeEmbedFailure Code = "EMBED_FAILURE"
)

// codeExit is the authoritative code->exit mapping from the plan.
var codeExit = map[Code]ExitCode{
	CodeUsage:             ExitUsage,
	CodeConfigInvalid:     ExitUsage,
	CodeInvalidKind:       ExitUsage,
	CodeNotInitialized:    ExitNotInitialized,
	CodeIndexMissing:      ExitNotInitialized,
	CodeIndexLocked:       ExitTransient,
	CodeSchemaTooNew:      ExitRuntime,
	CodeNotFound:          ExitRuntime,
	CodeStoreError:        ExitRuntime,
	CodeDependencyMissing: ExitRuntime,
	CodeRuntimeMissing:    ExitRuntime,
	CodeEmbedFailure:      ExitRuntime,
}

// Exit returns the process exit code for this error code. Unknown codes map
// to ExitRuntime (fail safe, never success).
func (c Code) Exit() ExitCode {
	if e, ok := codeExit[c]; ok {
		return e
	}
	return ExitRuntime
}

// Error is the canonical domain error. It carries a stable Code, a
// human-readable Message, and an optional actionable Hint.
type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

func (e *Error) Error() string { return e.Message }

// Errorf constructs an *Error with a formatted message.
func Errorf(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// WithHint returns a copy of the error with the given hint set.
func (e *Error) WithHint(hint string) *Error {
	return &Error{Code: e.Code, Message: e.Message, Hint: hint}
}

// AsError coerces any error into an *Error. If err already wraps an *Error it
// is returned as-is; otherwise it becomes a CodeStoreError (the safe default
// for unexpected failures).
func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	var ce *Error
	if errors.As(err, &ce) {
		return ce
	}
	return &Error{Code: CodeStoreError, Message: err.Error()}
}
