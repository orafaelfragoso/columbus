// Package knowledge is the unified durable-knowledge surface behind the
// `columbus memory <add|update|remove|list> <kind>` command. It presents one
// manager and one result model over every kind of durable knowledge —
// epics, stories, tasks (the work hierarchy) and context entries (free-form
// memories) — plus a read-only tag view. The richer per-kind engines in
// internal/work and internal/memory remain the storage layer; this package is
// the single coherent API and projection the CLI talks to.
package knowledge

import (
	"strings"

	"github.com/orafaelfragoso/columbus/internal/clock"
	"github.com/orafaelfragoso/columbus/internal/contract"
	"github.com/orafaelfragoso/columbus/internal/memory"
	"github.com/orafaelfragoso/columbus/internal/store"
	"github.com/orafaelfragoso/columbus/internal/work"
)

// Kinds is the unified knowledge vocabulary. epic/story/task form the work
// hierarchy; context is a free-form memory entry; tag is a read-only view over
// the tags attached to any entity.
var Kinds = []string{"epic", "story", "task", "context", "tag"}

// Manager is the single entry point for unified knowledge operations. It owns no
// state beyond the store, clock and working directory and delegates to the work
// and memory engines.
type Manager struct {
	DB      *store.DB
	Clock   clock.Clock
	WorkDir string
}

func (m *Manager) work() *work.Manager { return &work.Manager{DB: m.DB, Clock: m.Clock} }
func (m *Manager) mem() *memory.Manager {
	return &memory.Manager{DB: m.DB, Clock: m.Clock, WorkDir: m.WorkDir}
}

// validKind reports whether kind is part of the unified vocabulary.
func validKind(kind string) bool {
	for _, k := range Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

func unknownKind(kind string) error {
	return &contract.Error{
		Code:    contract.CodeInvalidKind,
		Message: "unknown knowledge kind: " + kind,
		Hint:    "one of: " + strings.Join(Kinds, ", "),
	}
}

// Statuses returns the work status vocabulary (for CLI help and validation).
func Statuses() []string { return work.Statuses }

func parentNotAllowed(kind string) error {
	return contract.Errorf(contract.CodeUsage, "%s entities have no parent", kind)
}

// requireMutableKind validates kind for a mutating verb (add/update/remove).
// tag is read-only; it is rejected here.
func requireMutableKind(kind string) error {
	if !validKind(kind) {
		return unknownKind(kind)
	}
	if kind == "tag" {
		return contract.Errorf(contract.CodeUsage,
			"tags are read-only here; attach them to an item with --tag")
	}
	return nil
}
