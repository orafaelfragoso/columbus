package extract

import (
	"regexp"
	"strings"
)

// todoRe matches common task markers. Matching is language-agnostic (best
// effort): we record any line containing a marker token. The full line is
// re-validated live at query time, so false positives here are cheap.
var todoRe = regexp.MustCompile(`\b(TODO|FIXME|HACK|XXX)\b`)

// scanTodos returns one Todo per source line containing a marker token.
func scanTodos(source []byte) []Todo {
	var todos []Todo
	line := 1
	for _, raw := range strings.Split(string(source), "\n") {
		if todoRe.MatchString(raw) {
			todos = append(todos, Todo{Line: line, Text: strings.TrimSpace(raw)})
		}
		line++
	}
	return todos
}
