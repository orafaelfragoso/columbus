// Package doctor runs environment and project health checks. It reports every
// check it can, distinguishing warnings (still usable -> exit 0) from hard
// failures (which set a recoverable or runtime exit code).
package doctor

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/orafaelfragoso/columbus/internal/config"
	"github.com/orafaelfragoso/columbus/internal/contract"
	"github.com/orafaelfragoso/columbus/internal/extract"
	"github.com/orafaelfragoso/columbus/internal/gitrepo"
	"github.com/orafaelfragoso/columbus/internal/render"
	"github.com/orafaelfragoso/columbus/internal/store"
)

// Status is a check outcome.
type Status string

const (
	StatusOK   Status = "ok"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
	StatusSkip Status = "skip"
)

// Check is a single diagnostic result.
type Check struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Detail string `json:"detail,omitempty"`
	Hint   string `json:"hint,omitempty"`
}

// Result is the typed doctor report.
type Result struct {
	Healthy bool    `json:"healthy"`
	Checks  []Check `json:"checks"`
}

func (Result) CommandName() string { return "doctor" }

func (r Result) RenderText(w io.Writer, o render.Options) error {
	for _, c := range r.Checks {
		fmt.Fprintf(w, "%s %-12s %s\n", glyph(c.Status, o.Color), c.Name, c.Detail)
		if c.Hint != "" {
			fmt.Fprintf(w, "    hint: %s\n", c.Hint)
		}
	}
	if r.Healthy {
		fmt.Fprintln(w, "\nhealthy")
	} else {
		fmt.Fprintln(w, "\nproblems found")
	}
	return nil
}

func (r Result) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# columbus doctor\n\nhealthy: %t\n\n", r.Healthy)
	for _, c := range r.Checks {
		fmt.Fprintf(w, "- **%s** [%s] %s\n", c.Name, c.Status, c.Detail)
	}
	return nil
}

func glyph(s Status, color bool) string {
	label := map[Status]string{StatusOK: "[ok]  ", StatusWarn: "[warn]", StatusFail: "[fail]", StatusSkip: "[skip]"}[s]
	if !color {
		return label
	}
	code := map[Status]string{StatusOK: "32", StatusWarn: "33", StatusFail: "31", StatusSkip: "90"}[s]
	return "\x1b[" + code + "m" + label + "\x1b[0m"
}

// Params are the inputs to Run.
type Params struct {
	WorkDir string
	Getenv  func(string) string
	Version string
	Ctx     context.Context

	// Embed is an optional probe that loads the embedding runtime (a dry run of
	// embed.New) and reports the model name and dimension. It is injected by the
	// cli layer so the doctor package stays free of the embed dependency.
	// A nil probe skips the runtime check.
	Embed func() (model string, dim int, err error)
}

func ctxOrBackground(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

// Run executes all checks. The returned Code is empty when the project is
// healthy (exit 0) or only warnings were raised; otherwise it is the canonical
// code whose exit status the caller should use.
func Run(p Params) (Result, contract.Code) {
	var checks []Check
	add := func(c Check) { checks = append(checks, c) }

	add(Check{Name: "columbus", Status: StatusOK, Detail: "version " + valueOr(p.Version, "dev")})
	add(checkBinary("git", []string{"git"}, true))
	add(checkBinary("ripgrep", []string{"rg"}, false))
	add(checkOptional("ast-grep", []string{"ast-grep", "sg"}))
	add(checkGrammars())

	cfgCheck, cfg, cfgCode := checkConfig(p)
	add(cfgCheck)

	if cfgCode == "" {
		add(checkDataDir(p))
		dbCheck, db := checkDatabase(p, cfg)
		add(dbCheck)
		if db != nil {
			defer db.Close()
			add(checkVec0(db))
			add(checkIndex(p, db))
			add(checkEmbedding(cfg, db))
		}
	} else {
		add(Check{Name: "database", Status: StatusSkip, Detail: "no project"})
		add(Check{Name: "vec0", Status: StatusSkip, Detail: "no project"})
		add(Check{Name: "index", Status: StatusSkip, Detail: "no project"})
		add(Check{Name: "embedding", Status: StatusSkip, Detail: "no project"})
	}
	add(checkRuntime(p, cfg))

	code := worstCode(checks, cfgCode)
	return Result{Healthy: code == "", Checks: checks}, code
}

func checkBinary(name string, candidates []string, required bool) Check {
	if path, ok := lookAny(candidates); ok {
		return Check{Name: name, Status: StatusOK, Detail: path}
	}
	if required {
		return Check{Name: name, Status: StatusFail, Detail: "not found on PATH", Hint: "install " + name}
	}
	return Check{Name: name, Status: StatusWarn, Detail: "not found (recommended)", Hint: "install " + name + " for the fast search path"}
}

func checkOptional(name string, candidates []string) Check {
	if path, ok := lookAny(candidates); ok {
		return Check{Name: name, Status: StatusOK, Detail: path}
	}
	return Check{Name: name, Status: StatusOK, Detail: "not installed (optional)"}
}

func checkGrammars() Check {
	reg, err := extract.NewRegistry()
	if err != nil {
		return Check{Name: "grammars", Status: StatusFail, Detail: err.Error()}
	}
	return Check{Name: "grammars", Status: StatusOK, Detail: fmt.Sprintf("%d languages loaded", len(reg.Languages()))}
}

// checkVec0 confirms the registered sqlite-vec (vec0) extension loads.
func checkVec0(db *store.DB) Check {
	v, err := db.VecVersion()
	if err != nil {
		return Check{Name: "vec0", Status: StatusFail, Detail: "sqlite-vec not loadable: " + err.Error()}
	}
	return Check{Name: "vec0", Status: StatusOK, Detail: "sqlite-vec " + v}
}

// expectedEmbedDim is the potion-code-16M output dimension. A probe returning
// anything else means a model/runtime mismatch.
const expectedEmbedDim = 256

// checkRuntime dry-runs the embedding runtime via the injected probe. Any
// failure, or a wrong dimension, is a hard failure.
func checkRuntime(p Params, _ config.Config) Check {
	if p.Embed == nil {
		return Check{Name: "runtime", Status: StatusSkip, Detail: "not probed"}
	}
	model, dim, err := p.Embed()
	if err != nil {
		ce := contract.AsError(err)
		return Check{Name: "runtime", Status: StatusFail, Detail: ce.Message, Hint: ce.Hint}
	}
	if dim != expectedEmbedDim {
		return Check{Name: "runtime", Status: StatusFail, Detail: fmt.Sprintf("model %q dim %d, want %d", model, dim, expectedEmbedDim)}
	}
	return Check{Name: "runtime", Status: StatusOK, Detail: fmt.Sprintf("%s (%d-d)", model, dim)}
}

func checkConfig(p Params) (Check, config.Config, contract.Code) {
	loaded, err := config.Load(filepath.Join(p.WorkDir, config.FileName))
	if err != nil {
		ce := contract.AsError(err)
		return Check{Name: "config", Status: StatusFail, Detail: ce.Message, Hint: ce.Hint}, config.Config{}, ce.Code
	}
	detail := "project " + loaded.Config.ProjectID
	status := StatusOK
	if len(loaded.Warnings) > 0 {
		status = StatusWarn
		detail += fmt.Sprintf(" (%d warning(s))", len(loaded.Warnings))
	}
	return Check{Name: "config", Status: status, Detail: detail}, loaded.Config, ""
}

func checkDataDir(p Params) Check {
	dir, err := config.DataDir(p.Getenv)
	if err != nil {
		ce := contract.AsError(err)
		return Check{Name: "data_dir", Status: StatusFail, Detail: ce.Message, Hint: ce.Hint}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Check{Name: "data_dir", Status: StatusFail, Detail: "not writable: " + err.Error()}
	}
	return Check{Name: "data_dir", Status: StatusOK, Detail: dir}
}

func checkDatabase(p Params, cfg config.Config) (Check, *store.DB) {
	dir, err := config.DataDir(p.Getenv)
	if err != nil {
		return Check{Name: "database", Status: StatusFail, Detail: err.Error()}, nil
	}
	paths := config.ProjectPaths(dir, cfg.ProjectID)
	if _, err := os.Stat(paths.DBPath); err != nil {
		return Check{Name: "database", Status: StatusWarn, Detail: "no database yet", Hint: "run columbus reindex"}, nil
	}
	db, err := store.Open(paths.DBPath)
	if err != nil {
		ce := contract.AsError(err)
		return Check{Name: "database", Status: StatusFail, Detail: ce.Message, Hint: ce.Hint}, nil
	}
	return Check{Name: "database", Status: StatusOK, Detail: fmt.Sprintf("schema v%d", store.LatestVersion)}, db
}

func checkIndex(p Params, db *store.DB) Check {
	meta, err := db.Meta().Get()
	if err != nil {
		return Check{Name: "index", Status: StatusFail, Detail: err.Error()}
	}
	if meta.FilesCount == 0 {
		return Check{Name: "index", Status: StatusWarn, Detail: "index is empty", Hint: "run columbus reindex"}
	}
	detail := fmt.Sprintf("%d files, %d symbols", meta.FilesCount, meta.SymbolsCount)

	git, gerr := gitrepo.DiscoverContext(ctxOrBackground(p.Ctx), p.WorkDir)
	if gerr == nil && git.IsRepo {
		if head, _ := git.HeadOID(); head != "" && meta.IndexedHead != "" && head != meta.IndexedHead {
			return Check{Name: "index", Status: StatusWarn, Detail: detail + " (stale: HEAD moved since last index)", Hint: "run columbus reindex"}
		}
	}
	if meta.Dirty {
		return Check{Name: "index", Status: StatusWarn, Detail: detail + " (working tree dirty)", Hint: "run columbus reindex"}
	}
	return Check{Name: "index", Status: StatusOK, Detail: detail}
}

// checkEmbedding reports the semantic-search runtime state: whether embeddings
// are enabled, the model the index was built with, and whether it matches the
// model this binary/config expects (a mismatch means search silently falls back
// to keyword until a full reindex).
func checkEmbedding(cfg config.Config, db *store.DB) Check {
	if !cfg.Embedding.Enabled {
		return Check{Name: "embedding", Status: StatusOK, Detail: "disabled in config"}
	}
	meta, err := db.Meta().Get()
	if err != nil {
		return Check{Name: "embedding", Status: StatusFail, Detail: err.Error()}
	}
	want := cfg.Embedding.Model
	if meta.EmbedModel == "" {
		return Check{Name: "embedding", Status: StatusWarn, Detail: "no embeddings yet (keyword fallback)", Hint: "run columbus reindex"}
	}
	if want != "" && meta.EmbedModel != want {
		return Check{
			Name: "embedding", Status: StatusWarn,
			Detail: fmt.Sprintf("model mismatch: index %q, config %q", meta.EmbedModel, want),
			Hint:   "run columbus reindex --full",
		}
	}
	return Check{Name: "embedding", Status: StatusOK, Detail: fmt.Sprintf("%s (%d-d)", meta.EmbedModel, meta.EmbedDim)}
}

// worstCode determines the exit code: the first hard failure governs. Warnings
// never fail the command.
func worstCode(checks []Check, cfgCode contract.Code) contract.Code {
	for _, c := range checks {
		if c.Status != StatusFail {
			continue
		}
		switch c.Name {
		case "git", "ripgrep":
			return contract.CodeDependencyMissing
		case "config":
			return cfgCode
		default:
			return contract.CodeStoreError
		}
	}
	return ""
}

func valueOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func lookAny(candidates []string) (string, bool) {
	for _, c := range candidates {
		if path, err := exec.LookPath(c); err == nil {
			return path, true
		}
	}
	return "", false
}
