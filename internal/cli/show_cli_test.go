package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestShowGraphE2E(t *testing.T) {
	work, data := t.TempDir(), t.TempDir()
	// Two TypeScript files where a imports b (relative, resolves) and an
	// external package; the graph should surface the import edge and ext node.
	files := map[string]string{
		"src/a.ts": "import { b } from './b';\nimport React from 'react';\nexport const a = () => b();\n",
		"src/b.ts": "export const b = () => 1;\n",
	}
	for rel, content := range files {
		full := filepath.Join(work, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	initProject(t, work, data)
	if _, _, code := runProj(t, work, data, "reindex"); code != 0 {
		t.Fatalf("index exit = %d", code)
	}

	out, errb, code := runProj(t, work, data, "graphs", "--json")
	if code != 0 {
		t.Fatalf("show graph exit = %d: %s", code, errb)
	}
	var g struct {
		Nodes []struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"nodes"`
		Edges []struct {
			From string `json:"from"`
			To   string `json:"to"`
			Type string `json:"type"`
		} `json:"edges"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal([]byte(out), &g); err != nil {
		t.Fatalf("not json: %v\n%s", err, out)
	}
	if g.Total < 2 {
		t.Fatalf("total = %d, want >= 2", g.Total)
	}
	var hasImport, hasExternal bool
	for _, e := range g.Edges {
		if e.Type == "import" {
			hasImport = true
		}
		if e.Type == "external" {
			hasExternal = true
		}
	}
	if !hasImport {
		t.Fatalf("expected an import edge in %+v", g.Edges)
	}
	if !hasExternal {
		t.Fatalf("expected an external edge (react) in %+v", g.Edges)
	}
	var hasExtNode bool
	for _, n := range g.Nodes {
		if n.Kind == "external" {
			hasExtNode = true
		}
	}
	if !hasExtNode {
		t.Fatal("expected an external package node")
	}
}

func TestShowGraphRoleFilterE2E(t *testing.T) {
	work, data := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(work, "svc.go"), []byte("package svc\n\nfunc New() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	initProject(t, work, data)
	runProj(t, work, data, "reindex")

	_, _, code := runProj(t, work, data, "graphs", "--role", "impl", "--max", "5", "--json")
	if code != 0 {
		t.Fatalf("show graph --role exit = %d", code)
	}
}
