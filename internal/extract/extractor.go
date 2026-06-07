package extract

import (
	"context"
	"sort"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/orafaelfragoso/columbus/internal/contract"
)

// Extractor produces the IR for one language.
type Extractor interface {
	Language() string
	Extract(source []byte) (Result, error)
}

// langSpec declares everything needed to extract one language: its grammar, an
// .scm query whose captures follow the @def.<kind> + @name convention, and
// small language-specific hooks for export/container derivation.
type langSpec struct {
	name  string
	lang  *sitter.Language
	query string
	// exported reports whether a definition is exported/public.
	exported func(name string, def *sitter.Node, src []byte) bool
	// containerOf returns the enclosing container name (class/interface/...),
	// or "" when top-level.
	containerOf func(def, name *sitter.Node, src []byte) string
}

// treeSitterExtractor is the generic, spec-driven Extractor.
type treeSitterExtractor struct {
	spec  langSpec
	query *sitter.Query
}

func newExtractor(spec langSpec) (*treeSitterExtractor, error) {
	q, err := sitter.NewQuery([]byte(spec.query), spec.lang)
	if err != nil {
		return nil, &contract.Error{
			Code:    contract.CodeStoreError,
			Message: "invalid tree-sitter query for " + spec.name + ": " + err.Error(),
		}
	}
	return &treeSitterExtractor{spec: spec, query: q}, nil
}

func (e *treeSitterExtractor) Language() string { return e.spec.name }

func (e *treeSitterExtractor) Extract(source []byte) (Result, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(e.spec.lang)
	tree, err := parser.ParseCtx(context.Background(), nil, source)
	if err != nil {
		return Result{}, &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
	}
	defer tree.Close()
	root := tree.RootNode()

	res := Result{}
	qc := sitter.NewQueryCursor()
	defer qc.Close()
	qc.Exec(e.query, root)

	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}
		m = qc.FilterPredicates(m, source)
		e.consumeMatch(m, source, &res)
	}

	res.Todos = scanTodos(source)
	dedupeSymbols(&res)
	return res, nil
}

// consumeMatch maps one query match to IR entries by inspecting capture names.
func (e *treeSitterExtractor) consumeMatch(m *sitter.QueryMatch, src []byte, res *Result) {
	var defNode, nameNode *sitter.Node
	var defKind Kind
	var importSpec, exportName string

	for _, c := range m.Captures {
		capName := e.query.CaptureNameForId(c.Index)
		switch {
		case capName == "name":
			nameNode = c.Node
		case capName == "import":
			importSpec = unquote(c.Node.Content(src))
		case capName == "export":
			exportName = c.Node.Content(src)
		case strings.HasPrefix(capName, "def."):
			defNode = c.Node
			defKind = Kind(strings.TrimPrefix(capName, "def."))
		}
	}

	if importSpec != "" {
		res.Imports = append(res.Imports, Import{Specifier: importSpec})
	}
	if exportName != "" {
		res.Exports = append(res.Exports, Export{Name: exportName})
	}
	if defNode == nil || nameNode == nil {
		return
	}

	name := nameNode.Content(src)
	sym := Symbol{
		Name:      name,
		Kind:      defKind,
		Signature: firstLine(defNode.Content(src)),
		StartLine: int(defNode.StartPoint().Row) + 1,
		EndLine:   int(defNode.EndPoint().Row) + 1,
	}
	if e.spec.containerOf != nil {
		sym.Container = e.spec.containerOf(defNode, nameNode, src)
	}
	// A function defined inside a container is a method (uniform across
	// languages: Python/JS nested defs, etc.).
	if sym.Kind == KindFunction && sym.Container != "" {
		sym.Kind = KindMethod
	}
	if e.spec.exported != nil {
		sym.Exported = e.spec.exported(name, defNode, src)
	}
	res.Symbols = append(res.Symbols, sym)
}

// kindPriority ranks kinds so that when overlapping query patterns match the
// same definition (e.g. a Go struct matches both the class and the generic
// type pattern), the most specific kind wins.
var kindPriority = map[Kind]int{
	KindClass:     6,
	KindInterface: 6,
	KindEnum:      6,
	KindMethod:    5,
	KindFunction:  5,
	KindHeading:   4,
	KindType:      3,
	KindConst:     2,
	KindVar:       1,
}

// dedupeSymbols collapses symbols sharing a (container, name) identity, keeping
// the highest-priority kind, then sorts deterministically by position.
func dedupeSymbols(res *Result) {
	best := map[string]int{} // identity -> index in out
	var out []Symbol
	for _, s := range res.Symbols {
		key := s.Container + "\x00" + s.Name
		if idx, ok := best[key]; ok {
			if kindPriority[s.Kind] > kindPriority[out[idx].Kind] {
				out[idx] = s
			}
			continue
		}
		best[key] = len(out)
		out = append(out, s)
	}
	res.Symbols = out
	sort.SliceStable(res.Symbols, func(i, j int) bool {
		if res.Symbols[i].StartLine != res.Symbols[j].StartLine {
			return res.Symbols[i].StartLine < res.Symbols[j].StartLine
		}
		return res.Symbols[i].Name < res.Symbols[j].Name
	})
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "`")
	s = strings.Trim(s, `"'`)
	return s
}
