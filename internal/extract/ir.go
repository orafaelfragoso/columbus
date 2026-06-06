// Package extract turns source files into a shared intermediate representation
// (symbols, imports, exports, todos) via embedded tree-sitter grammars driven
// by per-language .scm queries. Adding a language is grammar + queries +
// extension mapping; no changes to the core extraction loop.
package extract

// Kind is the normalized, cross-language symbol kind.
type Kind string

const (
	KindFunction  Kind = "function"
	KindMethod    Kind = "method"
	KindClass     Kind = "class"
	KindInterface Kind = "interface"
	KindType      Kind = "type"
	KindConst     Kind = "const"
	KindVar       Kind = "var"
	KindEnum      Kind = "enum"
	KindHeading   Kind = "heading"
)

// Symbol is a definition discovered in a file. Line fields are derived for
// convenience but are NOT authoritative (the store does not persist them; exact
// ranges are resolved live at query time).
type Symbol struct {
	Name      string `json:"name"`
	Kind      Kind   `json:"kind"`
	Container string `json:"container,omitempty"`
	Signature string `json:"signature,omitempty"`
	Exported  bool   `json:"exported"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

// Import is a raw import specifier (best-effort; resolution happens later).
type Import struct {
	Specifier string `json:"specifier"`
}

// Export is a named export.
type Export struct {
	Name string `json:"name"`
}

// Todo is a TODO/FIXME-style marker found in the source.
type Todo struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

// Result is the IR for a single file.
type Result struct {
	Symbols []Symbol `json:"symbols"`
	Imports []Import `json:"imports"`
	Exports []Export `json:"exports"`
	Todos   []Todo   `json:"todos"`
}
