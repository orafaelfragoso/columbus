package extract

import (
	"path/filepath"
	"strings"
	"unicode"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/javascript"
	markdown "github.com/smacker/go-tree-sitter/markdown/tree-sitter-markdown"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

// Registry maps file extensions to language extractors.
type Registry struct {
	byExt map[string]Extractor
}

// NewRegistry builds the registry of all V1 bundled languages. It compiles each
// language's query once; a malformed query is a programming error surfaced here.
func NewRegistry() (*Registry, error) {
	r := &Registry{byExt: map[string]Extractor{}}
	for _, b := range builders() {
		ex, err := newExtractor(b.spec)
		if err != nil {
			return nil, err
		}
		for _, ext := range b.exts {
			r.byExt[ext] = ex
		}
	}
	return r, nil
}

// ForPath returns the extractor for a file path, if any language matches.
func (r *Registry) ForPath(path string) (Extractor, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	ex, ok := r.byExt[ext]
	return ex, ok
}

// Languages returns the distinct language names the registry can handle.
func (r *Registry) Languages() []string {
	seen := map[string]bool{}
	var out []string
	for _, ex := range r.byExt {
		if !seen[ex.Language()] {
			seen[ex.Language()] = true
			out = append(out, ex.Language())
		}
	}
	return out
}

type builder struct {
	spec langSpec
	exts []string
}

func builders() []builder {
	return []builder{
		{spec: goSpec(), exts: []string{".go"}},
		{spec: tsSpec("typescript", typescript.GetLanguage()), exts: []string{".ts", ".mts", ".cts"}},
		{spec: tsSpec("tsx", tsx.GetLanguage()), exts: []string{".tsx"}},
		{spec: jsSpec("javascript", javascript.GetLanguage()), exts: []string{".js", ".jsx", ".mjs", ".cjs"}},
		{spec: pySpec(), exts: []string{".py", ".pyi"}},
		{spec: mdSpec(), exts: []string{".md", ".markdown"}},
	}
}

// ---- Go ----

func goSpec() langSpec {
	return langSpec{
		name: "go",
		lang: golang.GetLanguage(),
		query: `
(function_declaration name: (identifier) @name) @def.function
(method_declaration name: (field_identifier) @name) @def.method
(type_spec name: (type_identifier) @name type: (struct_type)) @def.class
(type_spec name: (type_identifier) @name type: (interface_type)) @def.interface
(type_spec name: (type_identifier) @name) @def.type
(const_spec name: (identifier) @name) @def.const
(var_spec name: (identifier) @name) @def.var
(import_spec path: (interpreted_string_literal) @import)
`,
		exported: func(name string, _ *sitter.Node, _ []byte) bool { return startsUpper(name) },
		containerOf: func(def, _ *sitter.Node, src []byte) string {
			if def.Type() != "method_declaration" {
				return ""
			}
			return goReceiverType(def, src)
		},
	}
}

// goReceiverType extracts the receiver type name (without pointer) from a Go
// method declaration.
func goReceiverType(method *sitter.Node, src []byte) string {
	recv := method.ChildByFieldName("receiver")
	if recv == nil {
		return ""
	}
	var typeNode *sitter.Node
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n == nil || typeNode != nil {
			return
		}
		if n.Type() == "type_identifier" {
			typeNode = n
			return
		}
		for i := 0; i < int(n.NamedChildCount()); i++ {
			walk(n.NamedChild(i))
		}
	}
	walk(recv)
	if typeNode == nil {
		return ""
	}
	return typeNode.Content(src)
}

// ---- TypeScript / TSX / JavaScript ----

// ecmaCommon holds the query patterns shared by JavaScript and TypeScript.
const ecmaCommon = `
(function_declaration name: (identifier) @name) @def.function
(generator_function_declaration name: (identifier) @name) @def.function
(method_definition name: (property_identifier) @name) @def.method
(lexical_declaration (variable_declarator name: (identifier) @name)) @def.const
(variable_declaration (variable_declarator name: (identifier) @name)) @def.var
(import_statement source: (string) @import)
(export_specifier name: (identifier) @export)
`

// tsSpec covers TypeScript and TSX: ECMAScript common patterns plus the
// TS-only type system (interfaces, type aliases, enums, abstract classes).
func tsSpec(name string, lang *sitter.Language) langSpec {
	return langSpec{
		name: name,
		lang: lang,
		query: ecmaCommon + `
(class_declaration name: (type_identifier) @name) @def.class
(abstract_class_declaration name: (type_identifier) @name) @def.class
(interface_declaration name: (type_identifier) @name) @def.interface
(type_alias_declaration name: (type_identifier) @name) @def.type
(enum_declaration name: (identifier) @name) @def.enum
`,
		exported:    ecmaExported,
		containerOf: ecmaContainer,
	}
}

// jsSpec covers JavaScript and JSX (no TS type system; class name is an
// identifier, not type_identifier).
func jsSpec(name string, lang *sitter.Language) langSpec {
	return langSpec{
		name: name,
		lang: lang,
		query: ecmaCommon + `
(class_declaration name: (identifier) @name) @def.class
`,
		exported:    ecmaExported,
		containerOf: ecmaContainer,
	}
}

func ecmaExported(name string, def *sitter.Node, _ []byte) bool {
	if def.Type() == "method_definition" {
		return !strings.HasPrefix(name, "_") && !strings.HasPrefix(name, "#")
	}
	return hasAncestorType(def, "export_statement")
}

func ecmaContainer(def, _ *sitter.Node, src []byte) string {
	return ancestorName(def, src, "class_declaration", "abstract_class_declaration", "class")
}

// ---- Python ----

func pySpec() langSpec {
	return langSpec{
		name: "python",
		lang: python.GetLanguage(),
		query: `
(function_definition name: (identifier) @name) @def.function
(class_definition name: (identifier) @name) @def.class
(import_statement name: (dotted_name) @import)
(import_from_statement module_name: (dotted_name) @import)
`,
		exported: func(name string, _ *sitter.Node, _ []byte) bool {
			return !strings.HasPrefix(name, "_")
		},
		containerOf: func(def, _ *sitter.Node, src []byte) string {
			return ancestorName(def, src, "class_definition")
		},
	}
}

// ---- Markdown ----

func mdSpec() langSpec {
	return langSpec{
		name: "markdown",
		lang: markdown.GetLanguage(),
		query: `
(atx_heading (inline) @name) @def.heading
(setext_heading (paragraph (inline) @name)) @def.heading
`,
		exported:    func(string, *sitter.Node, []byte) bool { return true },
		containerOf: func(*sitter.Node, *sitter.Node, []byte) string { return "" },
	}
}

// ---- shared helpers ----

func startsUpper(s string) bool {
	for _, r := range s {
		return unicode.IsUpper(r)
	}
	return false
}

func hasAncestorType(n *sitter.Node, types ...string) bool {
	for p := n.Parent(); p != nil; p = p.Parent() {
		for _, t := range types {
			if p.Type() == t {
				return true
			}
		}
	}
	return false
}

// ancestorName returns the name of the nearest ancestor whose type is in types.
func ancestorName(n *sitter.Node, src []byte, types ...string) string {
	for p := n.Parent(); p != nil; p = p.Parent() {
		for _, t := range types {
			if p.Type() == t {
				if name := p.ChildByFieldName("name"); name != nil {
					return name.Content(src)
				}
			}
		}
	}
	return ""
}
