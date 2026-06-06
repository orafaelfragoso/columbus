package store

import (
	"errors"
	"testing"

	sqlite3 "github.com/mattn/go-sqlite3"

	"github.com/rafaelfragoso/columbus/internal/contract"
)

// seedTwoFiles writes an implementation file with one symbol and a test file,
// returning their file ids.
func seedTwoFiles(t *testing.T, db *DB) (implID, testID int64) {
	t.Helper()
	err := db.WithTx(func(tx *Tx) error {
		if _, e := tx.PutFile(
			FileRecord{Path: "a.go", Language: "go", Package: "a", Role: "impl", BlobOID: "oid1", GrainEligible: true},
			[]SymbolRecord{{Name: "ParseConfig", Kind: "function", Signature: "func ParseConfig()", Exported: true}},
			[]string{"./b"}, []string{"ParseConfig"}, []TodoRecord{{Line: 3, Text: "TODO: x"}},
		); e != nil {
			return e
		}
		if _, e := tx.PutFile(
			FileRecord{Path: "a_test.go", Language: "go", Package: "a", Role: "test", ContentSHA256: "sha2"},
			nil, nil, nil, nil,
		); e != nil {
			return e
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	impl, _, _ := db.FileByPath("a.go")
	test, _, _ := db.FileByPath("a_test.go")
	return impl.ID, test.ID
}

func TestIndexRepoRoundTrip(t *testing.T) {
	db := openTemp(t)
	implID, _ := seedTwoFiles(t, db)

	hashes, err := db.FileHashes()
	if err != nil {
		t.Fatalf("FileHashes: %v", err)
	}
	if hashes["a.go"] != "oid1" || hashes["a_test.go"] != "sha2" {
		t.Fatalf("hashes = %v (blob_oid preferred over sha256)", hashes)
	}

	files, err := db.AllFiles()
	if err != nil || len(files) != 2 {
		t.Fatalf("AllFiles = %v, %v", files, err)
	}

	imports, err := db.AllImports()
	if err != nil || len(imports) != 1 || imports[0].Specifier != "./b" {
		t.Fatalf("AllImports = %v, %v", imports, err)
	}

	f, ok, err := db.FileByID(implID)
	if err != nil || !ok || f.Path != "a.go" {
		t.Fatalf("FileByID = %v ok=%v err=%v", f, ok, err)
	}

	syms, err := db.SymbolsByName("ParseConfig")
	if err != nil || len(syms) != 1 || !syms[0].Exported {
		t.Fatalf("SymbolsByName = %v, %v", syms, err)
	}
	inFile, err := db.SymbolsInFile(implID)
	if err != nil || len(inFile) != 1 {
		t.Fatalf("SymbolsInFile = %v, %v", inFile, err)
	}
	byID, ok, err := db.SymbolByID(syms[0].ID)
	if err != nil || !ok || byID.Name != "ParseConfig" {
		t.Fatalf("SymbolByID = %v ok=%v err=%v", byID, ok, err)
	}
}

func TestSuggestAndFTS(t *testing.T) {
	db := openTemp(t)
	seedTwoFiles(t, db)

	paths, err := db.SuggestPaths("a_te", 5)
	if err != nil || len(paths) != 1 || paths[0] != "a_test.go" {
		t.Fatalf("SuggestPaths = %v, %v", paths, err)
	}
	names, err := db.SuggestSymbols("ParseConf", 5)
	if err != nil || len(names) != 1 || names[0] != "ParseConfig" {
		t.Fatalf("SuggestSymbols = %v, %v", names, err)
	}
	hits, err := db.SearchCodeFTS("ParseConfig", 10)
	if err != nil || len(hits) == 0 {
		t.Fatalf("SearchCodeFTS = %v, %v", hits, err)
	}
}

func TestGraphRoundTrip(t *testing.T) {
	db := openTemp(t)
	implID, testID := seedTwoFiles(t, db)

	if err := db.WithTx(func(tx *Tx) error {
		return tx.ReplaceGraph([][2]int64{{testID, implID}}, [][2]int64{{implID, testID}})
	}); err != nil {
		t.Fatalf("ReplaceGraph: %v", err)
	}

	importedBy, err := db.ImportedBy(implID)
	if err != nil || len(importedBy) != 1 || importedBy[0] != "a_test.go" {
		t.Fatalf("ImportedBy = %v, %v", importedBy, err)
	}
	importsOf, err := db.ImportsOf(testID)
	if err != nil || len(importsOf) != 1 || importsOf[0] != "a.go" {
		t.Fatalf("ImportsOf = %v, %v", importsOf, err)
	}
	tests, err := db.TestsOf(implID)
	if err != nil || len(tests) != 1 || tests[0] != "a_test.go" {
		t.Fatalf("TestsOf = %v, %v", tests, err)
	}
	count, err := db.ImportedByCount(implID)
	if err != nil || count != 1 {
		t.Fatalf("ImportedByCount = %d, %v", count, err)
	}

	edges, err := db.AllDepEdges()
	if err != nil || len(edges) != 1 || edges[0].From != "a_test.go" || edges[0].To != "a.go" {
		t.Fatalf("AllDepEdges = %v, %v", edges, err)
	}
	links, err := db.AllTestLinks()
	if err != nil || len(links) != 1 || links[0].From != "a.go" || links[0].To != "a_test.go" {
		t.Fatalf("AllTestLinks = %v, %v", links, err)
	}
	// a.go imports "./b" which never resolved to an indexed file.
	unresolved, err := db.UnresolvedImports()
	if err != nil || len(unresolved) != 1 || unresolved[0].Path != "a.go" || unresolved[0].Specifier != "./b" {
		t.Fatalf("UnresolvedImports = %v, %v", unresolved, err)
	}
}

func TestDeleteAndClearIndex(t *testing.T) {
	db := openTemp(t)
	seedTwoFiles(t, db)

	if err := db.WithTx(func(tx *Tx) error { return tx.DeleteFileByPath("a_test.go") }); err != nil {
		t.Fatalf("DeleteFileByPath: %v", err)
	}
	if _, ok, _ := db.FileByPath("a_test.go"); ok {
		t.Fatal("a_test.go should be gone after DeleteFileByPath")
	}

	if err := db.WithTx(func(tx *Tx) error {
		if err := tx.ClearIndex(); err != nil {
			return err
		}
		n, err := tx.CountFiles()
		if err != nil {
			return err
		}
		if n != 0 {
			t.Errorf("CountFiles after ClearIndex = %d, want 0", n)
		}
		return nil
	}); err != nil {
		t.Fatalf("ClearIndex tx: %v", err)
	}
}

func TestSetIndexState(t *testing.T) {
	db := openTemp(t)
	seedTwoFiles(t, db)
	err := db.WithTx(func(tx *Tx) error {
		files, e := tx.CountFiles()
		if e != nil {
			return e
		}
		syms, e := tx.CountSymbols()
		if e != nil {
			return e
		}
		return tx.SetIndexState("headoid", true, files, syms, "2026-06-06T00:00:00Z")
	})
	if err != nil {
		t.Fatalf("SetIndexState tx: %v", err)
	}
	meta, err := db.Meta().Get()
	if err != nil {
		t.Fatalf("Meta.Get: %v", err)
	}
	if meta.IndexedHead != "headoid" || meta.FilesCount != 2 || meta.SymbolsCount != 1 {
		t.Fatalf("index meta = %+v", meta)
	}
}

func TestMemoryRepoRoundTrip(t *testing.T) {
	db := openTemp(t)

	var id int64
	err := db.WithTx(func(tx *Tx) error {
		var e error
		if id, e = tx.NextMemSeq(); e != nil {
			return e
		}
		if e = tx.InsertMemory(id, "decision", "use WAL", "we chose WAL mode", "t0", "t0"); e != nil {
			return e
		}
		if e = tx.AddTag(id, "storage"); e != nil {
			return e
		}
		if e = tx.AddEvidence(id, "store.go", 1, 10, "oidx"); e != nil {
			return e
		}
		if e = tx.AddLink(id, "file", "store.go"); e != nil {
			return e
		}
		return tx.ReindexMemoryFTS(id, "use WAL", "we chose WAL mode", []string{"storage"})
	})
	if err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	full, ok, err := db.MemoryFull(id)
	if err != nil || !ok {
		t.Fatalf("MemoryFull ok=%v err=%v", ok, err)
	}
	if full.Title != "use WAL" || len(full.Tags) != 1 || len(full.Evidence) != 1 || len(full.Links) != 1 {
		t.Fatalf("MemoryFull = %+v", full)
	}

	brief, ok, err := db.MemoryBriefByID(id)
	if err != nil || !ok || brief.Kind != "decision" {
		t.Fatalf("MemoryBriefByID = %+v ok=%v err=%v", brief, ok, err)
	}

	list, err := db.ListMemories("decision", "storage")
	if err != nil || len(list) != 1 {
		t.Fatalf("ListMemories = %v, %v", list, err)
	}
	ids, err := db.AllMemoryIDs()
	if err != nil || len(ids) != 1 || ids[0] != id {
		t.Fatalf("AllMemoryIDs = %v, %v", ids, err)
	}
	linked, err := db.MemoriesForTarget("file", "store.go")
	if err != nil || len(linked) != 1 {
		t.Fatalf("MemoriesForTarget = %v, %v", linked, err)
	}
	matches, err := db.SearchMemoryFTS("WAL", 10)
	if err != nil || len(matches) != 1 {
		t.Fatalf("SearchMemoryFTS = %v, %v", matches, err)
	}

	if err := db.WithTx(func(tx *Tx) error {
		if e := tx.UpdateMemory(id, "decision", "use WAL mode", "updated body", "t1"); e != nil {
			return e
		}
		if e := tx.RemoveTag(id, "storage"); e != nil {
			return e
		}
		if e := tx.RemoveLink(id, "file", "store.go"); e != nil {
			return e
		}
		return tx.RemoveEvidence(id, "store.go", 1, 10)
	}); err != nil {
		t.Fatalf("mutate memory: %v", err)
	}
	full, _, _ = db.MemoryFull(id)
	if full.Title != "use WAL mode" || len(full.Tags) != 0 || len(full.Links) != 0 || len(full.Evidence) != 0 {
		t.Fatalf("after mutation = %+v", full)
	}

	if err := db.WithTx(func(tx *Tx) error { return tx.DeleteMemory(id) }); err != nil {
		t.Fatalf("DeleteMemory: %v", err)
	}
	if _, ok, _ := db.MemoryBriefByID(id); ok {
		t.Fatal("memory should be gone after DeleteMemory")
	}
}

func TestSetMemSeqAtLeast(t *testing.T) {
	db := openTemp(t)
	if err := db.WithTx(func(tx *Tx) error { return tx.SetMemSeqAtLeast(50) }); err != nil {
		t.Fatalf("SetMemSeqAtLeast: %v", err)
	}
	n, err := db.Meta().NextMemSeq()
	if err != nil {
		t.Fatalf("NextMemSeq: %v", err)
	}
	if n != 51 {
		t.Fatalf("NextMemSeq after SetMemSeqAtLeast(50) = %d, want 51", n)
	}
}

func TestMapLockErr(t *testing.T) {
	if mapLockErr(nil) != nil {
		t.Error("nil error should map to nil")
	}

	passthrough := &contract.Error{Code: contract.CodeNotFound, Message: "x"}
	if got := mapLockErr(passthrough); !errors.Is(got, passthrough) {
		t.Error("existing contract error should pass through unchanged")
	}

	var ce *contract.Error
	if got := mapLockErr(sqlite3.Error{Code: sqlite3.ErrBusy}); !errors.As(got, &ce) || ce.Code != contract.CodeIndexLocked {
		t.Errorf("busy error = %v, want INDEX_LOCKED", got)
	}
	if got := mapLockErr(errors.New("plain boom")); !errors.As(got, &ce) || ce.Code != contract.CodeStoreError {
		t.Errorf("plain error = %v, want STORE_ERROR", got)
	}
}
