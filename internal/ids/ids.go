// Package ids provides an injectable source of non-deterministic identifiers
// (the project_id minted at init). Memory ids are monotonic and come from the
// store's counter, not from here.
package ids

import (
	"crypto/rand"
	"encoding/hex"
)

// Source mints identifiers.
type Source interface {
	// ProjectID returns a fresh project identifier: "proj_" + 16 hex chars
	// (8 random bytes).
	ProjectID() string
}

// Crypto is the production source backed by crypto/rand.
type Crypto struct{}

func (Crypto) ProjectID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure is catastrophic and not recoverable here.
		panic("ids: crypto/rand failed: " + err.Error())
	}
	return "proj_" + hex.EncodeToString(b[:])
}

// Fixed is a deterministic source for tests.
type Fixed struct{ ID string }

func (f Fixed) ProjectID() string { return f.ID }
