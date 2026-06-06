// Package clock provides an injectable time source so domain code never reads
// the wall clock directly (required for deterministic tests).
package clock

import "time"

// Clock yields the current time.
type Clock interface {
	Now() time.Time
}

// System is the production clock backed by time.Now.
type System struct{}

func (System) Now() time.Time { return time.Now() }

// Fixed is a deterministic clock for tests.
type Fixed struct{ T time.Time }

func (f Fixed) Now() time.Time { return f.T }
