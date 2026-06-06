package extract

import "testing"

func extractPath(t *testing.T, path, src string) Result {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	ex, ok := r.ForPath(path)
	if !ok {
		t.Fatalf("no extractor for %s", path)
	}
	res, err := ex.Extract([]byte(src))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	return res
}

// findSym returns the symbol matching name (and optional container), or fails.
func findSym(t *testing.T, res Result, name, container string) Symbol {
	t.Helper()
	for _, s := range res.Symbols {
		if s.Name == name && s.Container == container {
			return s
		}
	}
	t.Fatalf("symbol %q (container %q) not found in %+v", name, container, res.Symbols)
	return Symbol{}
}

func TestGoExtraction(t *testing.T) {
	src := `package foo

import "fmt"

// TODO: tidy this up
type Greeter interface {
	Greet() string
}

type Service struct {
	name string
}

func (s *Service) Greet() string { return "hi " + s.name }

func New(name string) *Service { return &Service{name: name} }

const Version = "1.0"

var defaultName = "world"
`
	res := extractPath(t, "svc.go", src)

	if s := findSym(t, res, "Greeter", ""); s.Kind != KindInterface || !s.Exported {
		t.Errorf("Greeter = %+v, want exported interface", s)
	}
	if s := findSym(t, res, "Service", ""); s.Kind != KindClass {
		t.Errorf("Service kind = %s, want class", s.Kind)
	}
	if s := findSym(t, res, "Greet", "Service"); s.Kind != KindMethod || !s.Exported {
		t.Errorf("Greet = %+v, want exported method on Service", s)
	}
	if s := findSym(t, res, "New", ""); s.Kind != KindFunction || !s.Exported {
		t.Errorf("New = %+v, want exported function", s)
	}
	if s := findSym(t, res, "Version", ""); s.Kind != KindConst {
		t.Errorf("Version kind = %s, want const", s.Kind)
	}
	if s := findSym(t, res, "defaultName", ""); s.Exported {
		t.Errorf("defaultName should be unexported")
	}
	if len(res.Imports) != 1 || res.Imports[0].Specifier != "fmt" {
		t.Errorf("imports = %+v, want [fmt]", res.Imports)
	}
	if len(res.Todos) != 1 {
		t.Errorf("todos = %+v, want 1", res.Todos)
	}
}

func TestTypeScriptExtraction(t *testing.T) {
	src := `import { z } from "zod";

export interface User {
	id: string;
}

export type ID = string;

export enum Color { Red, Green }

export class Repo {
	find(id: ID): User | undefined { return undefined; }
	private _cache(): void {}
}

export const make = (): Repo => new Repo();

function internalHelper(): void {}
`
	res := extractPath(t, "user.ts", src)

	if s := findSym(t, res, "User", ""); s.Kind != KindInterface || !s.Exported {
		t.Errorf("User = %+v, want exported interface", s)
	}
	if s := findSym(t, res, "ID", ""); s.Kind != KindType {
		t.Errorf("ID kind = %s, want type", s.Kind)
	}
	if s := findSym(t, res, "Color", ""); s.Kind != KindEnum {
		t.Errorf("Color kind = %s, want enum", s.Kind)
	}
	if s := findSym(t, res, "Repo", ""); s.Kind != KindClass || !s.Exported {
		t.Errorf("Repo = %+v, want exported class", s)
	}
	if s := findSym(t, res, "find", "Repo"); s.Kind != KindMethod || !s.Exported {
		t.Errorf("find = %+v, want exported method on Repo", s)
	}
	if s := findSym(t, res, "_cache", "Repo"); s.Exported {
		t.Errorf("_cache should be unexported")
	}
	if s := findSym(t, res, "make", ""); !s.Exported {
		t.Errorf("make should be exported (export const)")
	}
	if s := findSym(t, res, "internalHelper", ""); s.Exported {
		t.Errorf("internalHelper should not be exported")
	}
	if len(res.Imports) != 1 || res.Imports[0].Specifier != "zod" {
		t.Errorf("imports = %+v, want [zod]", res.Imports)
	}
}

func TestPythonExtraction(t *testing.T) {
	src := `import os
from collections import defaultdict

class Animal:
	def speak(self):
		return "..."

	def _private(self):
		pass

def main():
	pass

def _helper():
	pass
`
	res := extractPath(t, "zoo.py", src)

	if s := findSym(t, res, "Animal", ""); s.Kind != KindClass || !s.Exported {
		t.Errorf("Animal = %+v, want exported class", s)
	}
	if s := findSym(t, res, "speak", "Animal"); s.Kind != KindMethod || !s.Exported {
		t.Errorf("speak = %+v, want exported method on Animal", s)
	}
	if s := findSym(t, res, "_private", "Animal"); s.Exported {
		t.Errorf("_private should be unexported")
	}
	if s := findSym(t, res, "main", ""); s.Kind != KindFunction {
		t.Errorf("main kind = %s, want function", s.Kind)
	}
	if s := findSym(t, res, "_helper", ""); s.Exported {
		t.Errorf("_helper should be unexported")
	}
	if len(res.Imports) != 2 {
		t.Errorf("imports = %+v, want 2", res.Imports)
	}
}

func TestMarkdownExtraction(t *testing.T) {
	src := "# Title\n\nSome text.\n\n## Section One\n\nMore.\n\n### Deep\n"
	res := extractPath(t, "doc.md", src)

	names := map[string]bool{}
	for _, s := range res.Symbols {
		if s.Kind != KindHeading {
			t.Errorf("markdown symbol %q kind = %s, want heading", s.Name, s.Kind)
		}
		names[s.Name] = true
	}
	for _, want := range []string{"Title", "Section One", "Deep"} {
		if !names[want] {
			t.Errorf("missing heading %q in %+v", want, res.Symbols)
		}
	}
}

func TestJavaScriptExtraction(t *testing.T) {
	src := `export function add(a, b) { return a + b; }

class Widget {
	render() {}
}

const PI = 3.14;
`
	res := extractPath(t, "app.js", src)
	if s := findSym(t, res, "add", ""); s.Kind != KindFunction || !s.Exported {
		t.Errorf("add = %+v, want exported function", s)
	}
	if s := findSym(t, res, "Widget", ""); s.Kind != KindClass {
		t.Errorf("Widget kind = %s, want class", s.Kind)
	}
	if s := findSym(t, res, "render", "Widget"); s.Kind != KindMethod {
		t.Errorf("render = %+v, want method on Widget", s)
	}
}

func TestUnsupportedExtensionHasNoExtractor(t *testing.T) {
	r, _ := NewRegistry()
	if _, ok := r.ForPath("data.bin"); ok {
		t.Error("expected no extractor for .bin")
	}
}
