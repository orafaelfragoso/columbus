package search

import "strings"

// Ranking weights. These are compiled constants in V1 (exposed in config only
// once stable). They sum to 1.0 so the final score lies in [0,1]. The score is
// documented as a RELATIVE ranking signal, not an absolute confidence.
const (
	wName    = 0.45 // symbol/file name match (exact > prefix > substring)
	wPath    = 0.13 // path / filename match
	wSig     = 0.12 // signature / identifier match
	wContent = 0.10 // live content-match density (M8; 0 on the metadata path)
	wCentral = 0.10 // graph centrality (imported_by, has-tests)
	wRole    = 0.05 // role weighting (impl > doc > test)
	wMemory  = 0.05 // memory-linkage boost
)

// signals carries every ranking input for one candidate. Candidate generators
// (FTS/rg) never contribute their internal scores; only these signals do.
type signals struct {
	name             string
	signature        string
	path             string
	role             string
	importedByCount  int
	hasTests         bool
	hasMemory        bool
	hasFailureMemory bool
	contentDensity   float64 // [0,1], from the live path
}

// feature is one named, weighted contribution to the score.
type feature struct {
	name   string
	weight float64
	value  float64
}

// scoreFeatures computes the individual features for a candidate against the
// query tokens.
func scoreFeatures(tokens []string, s signals) []feature {
	return []feature{
		{"name", wName, nameMatch(tokens, s.name)},
		{"path", wPath, tokenCoverage(tokens, s.path)},
		{"signature", wSig, tokenCoverage(tokens, s.signature)},
		{"content", wContent, clamp01(s.contentDensity)},
		{"centrality", wCentral, centrality(s)},
		{"role", wRole, roleWeight(s.role)},
		{"memory", wMemory, boolVal(s.hasMemory)},
	}
}

// score returns the weighted sum in [0,1].
func score(tokens []string, s signals) float64 {
	var total float64
	for _, f := range scoreFeatures(tokens, s) {
		total += f.weight * f.value
	}
	return total
}

// why returns a templated explanation from the dominant feature.
func why(tokens []string, s signals) string {
	feats := scoreFeatures(tokens, s)
	best := feats[0]
	for _, f := range feats[1:] {
		if f.weight*f.value > best.weight*best.value {
			best = f
		}
	}
	if best.value == 0 {
		return "weak match"
	}
	switch best.name {
	case "name":
		switch nameMatch(tokens, s.name) {
		case 1.0:
			return "exact name match"
		case 0.7:
			return "name prefix match"
		default:
			return "name contains query"
		}
	case "path":
		return "path match"
	case "signature":
		return "signature match"
	case "content":
		return "dense content match"
	case "centrality":
		if s.importedByCount > 0 {
			return "frequently imported"
		}
		return "has tests"
	case "role":
		return "implementation file"
	case "memory":
		return "linked to project memory"
	default:
		return "match"
	}
}

// riskLevel is a crude, documented heuristic hint (not a guarantee).
func riskLevel(s signals) string {
	switch {
	case s.hasFailureMemory:
		return "high"
	case s.importedByCount >= 5:
		return "medium"
	default:
		return "low"
	}
}

// nameMatch scores how well any query token matches the name: exact 1.0, prefix
// 0.7, substring 0.4, none 0. The whole query (joined) is also tried for exact.
func nameMatch(tokens []string, name string) float64 {
	if name == "" || len(tokens) == 0 {
		return 0
	}
	low := strings.ToLower(name)
	joined := strings.Join(tokens, "")
	if low == joined {
		return 1.0
	}
	best := 0.0
	for _, tok := range tokens {
		switch {
		case low == tok:
			return 1.0
		case strings.HasPrefix(low, tok):
			best = max(best, 0.7)
		case strings.Contains(low, tok):
			best = max(best, 0.4)
		}
	}
	return best
}

// tokenCoverage returns the fraction of query tokens present in text.
func tokenCoverage(tokens []string, text string) float64 {
	if len(tokens) == 0 || text == "" {
		return 0
	}
	low := strings.ToLower(text)
	hits := 0
	for _, tok := range tokens {
		if strings.Contains(low, tok) {
			hits++
		}
	}
	return float64(hits) / float64(len(tokens))
}

func centrality(s signals) float64 {
	v := clamp01(float64(s.importedByCount) / 5.0)
	if s.hasTests {
		v = clamp01(v + 0.3)
	}
	return v
}

func roleWeight(role string) float64 {
	switch role {
	case "impl":
		return 1.0
	case "doc":
		return 0.6
	case "test":
		return 0.4
	default:
		return 0.5
	}
}

func boolVal(b bool) float64 {
	if b {
		return 1.0
	}
	return 0
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// tokenize splits a query into lowercased alphanumeric tokens.
func tokenize(q string) []string {
	var tokens []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, strings.ToLower(cur.String()))
			cur.Reset()
		}
	}
	for _, r := range q {
		if isWordRune(r) {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return tokens
}

func isWordRune(r rune) bool {
	return r == '_' ||
		(r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9')
}
