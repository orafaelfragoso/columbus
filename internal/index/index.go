// Package index builds and maintains the metadata index: file-set selection,
// content-hash change detection, parallel parse with a single serialized
// writer, and the atomic transaction that makes an index run all-or-nothing.
package index

import (
	"context"
	"runtime"
	"sync"

	"github.com/rafaelfragoso/columbus/internal/clock"
	"github.com/rafaelfragoso/columbus/internal/extract"
	"github.com/rafaelfragoso/columbus/internal/gitrepo"
	"github.com/rafaelfragoso/columbus/internal/store"
)

// Mode selects an indexing strategy.
type Mode int

const (
	// ModeIncremental (default) diffs the working tree vs the indexed state,
	// reindexes changed/added files and drops deleted ones.
	ModeIncremental Mode = iota
	// ModeFull reindexes everything from scratch (memories preserved).
	ModeFull
	// ModeChanged only reindexes files dirty in the working tree (fast path).
	ModeChanged
	// ModeClean drops all index data, preserving config and memories.
	ModeClean
	// ModeStatus reports state without writing.
	ModeStatus
)

func (m Mode) String() string {
	switch m {
	case ModeFull:
		return "full"
	case ModeChanged:
		return "changed"
	case ModeClean:
		return "clean"
	case ModeStatus:
		return "status"
	default:
		return "incremental"
	}
}

// Indexer runs indexing operations against a store.
type Indexer struct {
	DB          *store.DB
	Registry    *extract.Registry
	WorkDir     string
	Clock       clock.Clock
	MaxFileSize int64
	Excludes    []string
	Concurrency int
	// Ctx cancels git subprocesses spawned during indexing. nil = background.
	Ctx context.Context
}

func (ix *Indexer) ctx() context.Context {
	if ix.Ctx != nil {
		return ix.Ctx
	}
	return context.Background()
}

// parsed is the parse output for a changed file, ready to write.
type parsed struct {
	record  store.FileRecord
	symbols []store.SymbolRecord
	imports []string
	exports []string
	todos   []store.TodoRecord
}

// outcome is the phase-1 result for one candidate.
type outcome struct {
	rel     string
	skip    string // "", "binary", "oversized", "unreadable"
	changed bool
	parsed  *parsed // non-nil when changed and (re)parsed
}

// Run executes the indexing operation for the given mode and returns stats.
func (ix *Indexer) Run(mode Mode) (IndexResult, error) {
	git, err := gitrepo.DiscoverContext(ix.ctx(), ix.WorkDir)
	if err != nil {
		return IndexResult{}, err
	}

	if mode == ModeClean {
		return ix.runClean(git)
	}

	cands, err := selectFiles(ix.WorkDir, git, ix.Excludes, mode == ModeChanged)
	if err != nil {
		return IndexResult{}, err
	}
	existing, err := ix.DB.FileHashes()
	if err != nil {
		return IndexResult{}, err
	}

	outcomes := ix.scan(cands, existing, mode == ModeFull, mode != ModeStatus)

	if mode == ModeStatus {
		return ix.summarizeStatus(git, cands, existing, outcomes)
	}
	res, err := ix.write(git, mode, cands, existing, outcomes)
	if err != nil {
		return IndexResult{}, err
	}
	if err := ix.resolveGraphEdges(); err != nil {
		return IndexResult{}, err
	}
	return res, nil
}

// resolveGraphEdges recomputes best-effort dependency edges and test links from
// the now-complete files/imports tables.
func (ix *Indexer) resolveGraphEdges() error {
	files, err := ix.DB.AllFiles()
	if err != nil {
		return err
	}
	imports, err := ix.DB.AllImports()
	if err != nil {
		return err
	}
	metas := make([]FileMeta, len(files))
	for i, f := range files {
		metas[i] = FileMeta{ID: f.ID, Path: f.Path, Role: f.Role, Language: f.Language}
	}
	rawImports := make([]ImportRow, len(imports))
	for i, im := range imports {
		rawImports[i] = ImportRow{FileID: im.FileID, Specifier: im.Specifier}
	}
	depEdges, testLinks := resolveGraph(metas, rawImports)

	edges := make([][2]int64, len(depEdges))
	for i, e := range depEdges {
		edges[i] = [2]int64{e.From, e.To}
	}
	links := make([][2]int64, len(testLinks))
	for i, l := range testLinks {
		links[i] = [2]int64{l.Impl, l.Test}
	}
	return ix.DB.WithTx(func(tx *store.Tx) error {
		return tx.ReplaceGraph(edges, links)
	})
}

// scan runs phase 1: read, hash, change-detect and (optionally) parse each
// candidate concurrently. Order of outcomes matches cands.
func (ix *Indexer) scan(cands []candidate, existing map[string]string, full, doParse bool) []outcome {
	workers := ix.Concurrency
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	resolver := newPackageResolver(ix.WorkDir)

	outcomes := make([]outcome, len(cands))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				outcomes[i] = ix.scanOne(cands[i], existing, full, doParse, resolver)
			}
		}()
	}
	for i := range cands {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return outcomes
}

func (ix *Indexer) scanOne(c candidate, existing map[string]string, full, doParse bool, resolver *packageResolver) outcome {
	content, err := readFile(ix.WorkDir, c.RelPath)
	if err != nil {
		return outcome{rel: c.RelPath, skip: "unreadable"}
	}
	if ix.MaxFileSize > 0 && int64(len(content)) > ix.MaxFileSize {
		return outcome{rel: c.RelPath, skip: "oversized"}
	}
	if isBinary(content) {
		return outcome{rel: c.RelPath, skip: "binary"}
	}

	var blobOID, sha, effective string
	if c.Tracked {
		blobOID = gitBlobOID(content)
		effective = blobOID
	} else {
		sha = sha256hex(content)
		effective = sha
	}

	changed := full || existing[c.RelPath] != effective
	o := outcome{rel: c.RelPath, changed: changed}
	if !changed || !doParse {
		return o
	}

	ex, ok := ix.Registry.ForPath(c.RelPath)
	rec := store.FileRecord{
		Path:          c.RelPath,
		Role:          deriveRole(c.RelPath),
		Package:       resolver.pkgFor(c.RelPath),
		BlobOID:       blobOID,
		ContentSHA256: sha,
		GrainEligible: ok,
	}
	p := &parsed{record: rec}
	if ok {
		rec.Language = ex.Language()
		p.record = rec
		if ir, err := ex.Extract(content); err == nil {
			p.symbols, p.imports, p.exports, p.todos = mapIR(ir)
		}
	}
	o.parsed = p
	return o
}

// write runs phase 2: all DB mutations inside one atomic transaction.
func (ix *Indexer) write(git gitrepo.Info, mode Mode, cands []candidate, existing map[string]string, outcomes []outcome) (IndexResult, error) {
	res := IndexResult{Mode: mode.String()}

	candSet := make(map[string]bool, len(cands))
	for _, c := range cands {
		candSet[c.RelPath] = true
	}

	err := ix.DB.WithTx(func(tx *store.Tx) error {
		if mode == ModeFull {
			if err := tx.ClearIndex(); err != nil {
				return err
			}
		}
		if mode == ModeIncremental {
			for path := range existing {
				if !candSet[path] {
					if err := tx.DeleteFileByPath(path); err != nil {
						return err
					}
					res.Deleted++
				}
			}
		}
		for _, o := range outcomes {
			switch {
			case o.skip == "binary":
				res.SkippedBinary++
			case o.skip == "oversized":
				res.SkippedOversized++
			case o.skip == "unreadable":
				res.SkippedUnreadable++
			case o.parsed != nil:
				n, err := tx.PutFile(o.parsed.record, o.parsed.symbols, o.parsed.imports, o.parsed.exports, o.parsed.todos)
				if err != nil {
					return err
				}
				res.Indexed++
				res.Symbols += n
			default:
				res.Unchanged++
			}
		}

		head, err := git.HeadOID()
		if err != nil {
			return err
		}
		dirty, err := git.IsDirty()
		if err != nil {
			return err
		}
		total, err := tx.CountFiles()
		if err != nil {
			return err
		}
		symCount, err := tx.CountSymbols()
		if err != nil {
			return err
		}
		res.TotalFiles = total
		res.IndexedHead = head
		res.Dirty = dirty
		return tx.SetIndexState(head, dirty, total, symCount, ix.Clock.Now().UTC().Format("2006-01-02T15:04:05Z"))
	})
	if err != nil {
		return IndexResult{}, err
	}
	return res, nil
}

func (ix *Indexer) runClean(git gitrepo.Info) (IndexResult, error) {
	res := IndexResult{Mode: ModeClean.String()}
	err := ix.DB.WithTx(func(tx *store.Tx) error {
		before, err := tx.CountFiles()
		if err != nil {
			return err
		}
		res.Deleted = before
		if err := tx.ClearIndex(); err != nil {
			return err
		}
		return tx.SetIndexState("", false, 0, 0, ix.Clock.Now().UTC().Format("2006-01-02T15:04:05Z"))
	})
	if err != nil {
		return IndexResult{}, err
	}
	return res, nil
}

func (ix *Indexer) summarizeStatus(git gitrepo.Info, cands []candidate, existing map[string]string, outcomes []outcome) (IndexResult, error) {
	res := IndexResult{Mode: ModeStatus.String(), StatusOnly: true}
	candSet := make(map[string]bool, len(cands))
	for _, c := range cands {
		candSet[c.RelPath] = true
	}
	for _, o := range outcomes {
		switch {
		case o.skip == "binary":
			res.SkippedBinary++
		case o.skip == "oversized":
			res.SkippedOversized++
		case o.skip == "unreadable":
			res.SkippedUnreadable++
		case o.changed:
			res.Indexed++ // "would index"
		default:
			res.Unchanged++
		}
	}
	for path := range existing {
		if !candSet[path] {
			res.Deleted++ // "would delete"
		}
	}
	head, err := git.HeadOID()
	if err != nil {
		return IndexResult{}, err
	}
	dirty, err := git.IsDirty()
	if err != nil {
		return IndexResult{}, err
	}
	res.IndexedHead = head
	res.Dirty = dirty
	res.TotalFiles = len(existing)
	return res, nil
}

func mapIR(ir extract.Result) (syms []store.SymbolRecord, imports, exports []string, todos []store.TodoRecord) {
	for _, s := range ir.Symbols {
		syms = append(syms, store.SymbolRecord{
			Name:      s.Name,
			Kind:      string(s.Kind),
			Container: s.Container,
			Signature: s.Signature,
			Exported:  s.Exported,
		})
	}
	for _, im := range ir.Imports {
		imports = append(imports, im.Specifier)
	}
	for _, e := range ir.Exports {
		exports = append(exports, e.Name)
	}
	for _, td := range ir.Todos {
		todos = append(todos, store.TodoRecord{Line: td.Line, Text: td.Text})
	}
	return
}
