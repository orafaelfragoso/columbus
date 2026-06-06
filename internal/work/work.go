// Package work owns the project's structured memory: epics and tasks. They are
// passive durable-knowledge entities alongside memories — the store records
// status, history (an append-only event log), comments, tags and references. It
// never drives, gates, or enforces transitions; status is just a recorded
// field with a fixed, validated vocabulary.
package work

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/rafaelfragoso/columbus/internal/clock"
	"github.com/rafaelfragoso/columbus/internal/contract"
	"github.com/rafaelfragoso/columbus/internal/store"
)

// Statuses is the fixed, shared status vocabulary. The CLI rejects unknown
// values but enforces no transition order (any -> any): validating the
// vocabulary is data validation, not orchestration.
var Statuses = []string{"todo", "in_progress", "blocked", "done", "cancelled"}

// StatusDefault is the status a freshly created epic or task starts in.
const StatusDefault = "todo"

func validStatus(s string) bool {
	for _, v := range Statuses {
		if v == s {
			return true
		}
	}
	return false
}

// FormatEpicID / FormatTaskID render a numeric id as epic_NNN / task_NNN.
func FormatEpicID(id int64) string { return fmt.Sprintf("epic_%03d", id) }
func FormatTaskID(id int64) string { return fmt.Sprintf("task_%03d", id) }

// ParseEpicID / ParseTaskID parse an epic_NNN / task_NNN identifier.
func ParseEpicID(id string) (int64, error) { return parseID(id, "epic_", "epic") }
func ParseTaskID(id string) (int64, error) { return parseID(id, "task_", "task") }

func parseID(id, prefix, label string) (int64, error) {
	v, err := strconv.ParseInt(strings.TrimPrefix(id, prefix), 10, 64)
	if err != nil || v <= 0 {
		return 0, contract.Errorf(contract.CodeUsage, "invalid %s id %q (want %sNNN)", label, id, prefix)
	}
	return v, nil
}

// Manager performs epic/task operations against a store. References resolve
// against the indexed store, so no working-tree path is needed.
type Manager struct {
	DB    *store.DB
	Clock clock.Clock
}

func (m *Manager) now() string {
	return m.Clock.Now().UTC().Format("2006-01-02T15:04:05Z")
}

func notFound(kind, id string) *contract.Error {
	return &contract.Error{Code: contract.CodeNotFound, Message: kind + " not found: " + id}
}

func invalidStatus(to string) *contract.Error {
	return contract.Errorf(contract.CodeUsage, "unknown status %q (one of: %s)", to, strings.Join(Statuses, ", "))
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
