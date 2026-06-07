package work

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/orafaelfragoso/columbus/internal/contract"
	"github.com/orafaelfragoso/columbus/internal/store"
)

// RefSpec is a parsed reference: a target type and the bare reference text.
type RefSpec struct {
	Type string // file | dir | memory | symbol
	Ref  string
}

var refTypes = map[string]bool{"file": true, "dir": true, "memory": true, "symbol": true}

// ParseRef parses a "<type>:<ref>" reference spec (used by --remove-ref).
func ParseRef(s string) (RefSpec, error) {
	typ, ref, ok := strings.Cut(s, ":")
	if !ok || ref == "" || !refTypes[typ] {
		return RefSpec{}, contract.Errorf(contract.CodeUsage,
			"invalid ref %q (want file:<path>, dir:<path>, memory:<mem_NNN> or symbol:<name>)", s)
	}
	return RefSpec{Type: typ, Ref: ref}, nil
}

// EpicRef adds and/or removes references on an epic. Unresolved targets are
// stored anyway and reported as drift warnings, never errors.
func (m *Manager) EpicRef(idStr string, add, remove []RefSpec) (EpicResult, error) {
	id, err := ParseEpicID(idStr)
	if err != nil {
		return EpicResult{}, err
	}
	warnings, err := m.applyRefs("epic", id, idStr, add, remove)
	if err != nil {
		return EpicResult{}, err
	}
	r, err := m.loadEpic(id)
	if err != nil {
		return EpicResult{}, err
	}
	r.Warnings = warnings
	return r, nil
}

// TaskRef adds and/or removes references on a task.
func (m *Manager) TaskRef(idStr string, add, remove []RefSpec) (TaskResult, error) {
	id, err := ParseTaskID(idStr)
	if err != nil {
		return TaskResult{}, err
	}
	warnings, err := m.applyRefs("task", id, idStr, add, remove)
	if err != nil {
		return TaskResult{}, err
	}
	r, err := m.loadTask(id)
	if err != nil {
		return TaskResult{}, err
	}
	r.Warnings = warnings
	return r, nil
}

// applyRefs validates inputs, computes drift warnings (reads must precede the
// transaction; the writer holds the single connection), then mutates.
func (m *Manager) applyRefs(ownerType string, id int64, idStr string, add, remove []RefSpec) ([]string, error) {
	if len(add)+len(remove) == 0 {
		return nil, contract.Errorf(contract.CodeUsage, "ref requires at least one --file/--dir/--memory/--symbol or --remove-ref")
	}
	if ok, err := m.ownerExists(ownerType, id); err != nil {
		return nil, err
	} else if !ok {
		return nil, notFound(ownerType, idStr)
	}
	var warnings []string
	for _, ref := range add {
		if !m.refResolves(ref) {
			warnings = append(warnings, fmt.Sprintf("ref %s:%s does not resolve (stored anyway)", ref.Type, ref.Ref))
		}
	}
	err := m.DB.WithTx(func(tx *store.Tx) error {
		for _, ref := range remove {
			if e := tx.RemoveWorkRef(ownerType, id, ref.Type, ref.Ref); e != nil {
				return e
			}
		}
		for _, ref := range add {
			if e := tx.AddWorkRef(ownerType, id, ref.Type, ref.Ref); e != nil {
				return e
			}
		}
		return nil
	})
	return warnings, err
}

// refResolves reports whether a reference target exists in the current index.
func (m *Manager) refResolves(ref RefSpec) bool {
	switch ref.Type {
	case "file":
		_, ok, _ := m.DB.FileByPath(ref.Ref)
		return ok
	case "dir":
		ok, _ := m.DB.HasFilesUnderDir(ref.Ref)
		return ok
	case "memory":
		memID, err := parseMemID(ref.Ref)
		if err != nil {
			return false
		}
		ok, _ := m.DB.MemoryExists(memID)
		return ok
	case "symbol":
		rows, _ := m.DB.SymbolsByName(ref.Ref)
		return len(rows) > 0
	}
	return false
}

func parseMemID(s string) (int64, error) {
	v, err := strconv.ParseInt(strings.TrimPrefix(s, "mem_"), 10, 64)
	if err != nil || v <= 0 {
		return 0, contract.Errorf(contract.CodeUsage, "invalid memory id %q (want mem_NNN)", s)
	}
	return v, nil
}

// EpicValidate scans every epic's references for drift (warnings only).
func (m *Manager) EpicValidate() (ValidateResult, error) {
	ids, err := m.DB.AllEpicIDs()
	if err != nil {
		return ValidateResult{}, err
	}
	res := ValidateResult{command: "epic"}
	for _, id := range ids {
		full, ok, err := m.DB.EpicFull(id)
		if err != nil {
			return ValidateResult{}, err
		}
		if !ok {
			continue
		}
		res.add(m.validateRefs(FormatEpicID(id), full.Title, full.Status, full.Refs))
	}
	res.finalize()
	return res, nil
}

// TaskValidate scans every task's references for drift (warnings only).
func (m *Manager) TaskValidate() (ValidateResult, error) {
	ids, err := m.DB.AllTaskIDs()
	if err != nil {
		return ValidateResult{}, err
	}
	res := ValidateResult{command: "task"}
	for _, id := range ids {
		full, ok, err := m.DB.TaskFull(id)
		if err != nil {
			return ValidateResult{}, err
		}
		if !ok {
			continue
		}
		res.add(m.validateRefs(FormatTaskID(id), full.Title, full.Status, full.Refs))
	}
	res.finalize()
	return res, nil
}

func (m *Manager) validateRefs(id, title, status string, refs []store.WorkRef) WorkValidation {
	entry := WorkValidation{ID: id, Title: title, Status: status}
	for _, ref := range refs {
		resolved := m.refResolves(RefSpec{Type: ref.TargetType, Ref: ref.TargetRef})
		entry.Refs = append(entry.Refs, RefStatus{TargetType: ref.TargetType, TargetRef: ref.TargetRef, Resolved: resolved})
		if !resolved {
			entry.Warnings = append(entry.Warnings, "ref "+ref.TargetType+":"+ref.TargetRef+" does not resolve")
		}
	}
	return entry
}
