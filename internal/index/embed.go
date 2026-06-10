package index

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"strings"

	"github.com/orafaelfragoso/columbus/internal/contract"
	"github.com/orafaelfragoso/columbus/internal/extract"
)

const (
	symbolOwner = "symbol"
	fileOwner   = "file"
)

// embedErr wraps an embedder failure as a contract error (exit ExitRuntime).
func embedErr(err error) error {
	return contract.Errorf(contract.CodeEmbedFailure, "embed chunks: %v", err)
}

// fileCapture is the pre-write snapshot of a file's stored symbol vectors,
// keyed by symbol identity. The embed phase uses it to carry unchanged vectors
// to the new symbol ids (write() deletes and reinserts changed files, so ids
// churn) and to delete the orphaned old rows.
type fileCapture struct {
	oldFileID    int64
	oldSymbolIDs []int64
	byIdentity   map[string]storedChunk
}

// storedChunk mirrors store.SymbolChunk's fields the embed phase needs.
type storedChunk struct {
	sha string
	vec []float32
}

// captureChunks snapshots existing vectors for files about to be rewritten
// (parsed) or, in incremental mode, deleted, before write() churns their ids.
func (ix *Indexer) captureChunks(outcomes []outcome, existing map[string]string, candSet map[string]bool) (map[string]fileCapture, error) {
	model := ix.Embedder.Model()
	caps := make(map[string]fileCapture)

	capture := func(path string) error {
		if _, done := caps[path]; done {
			return nil
		}
		f, ok, err := ix.DB.FileByPath(path)
		if err != nil {
			return err
		}
		if !ok {
			caps[path] = fileCapture{}
			return nil
		}
		chunks, err := ix.DB.FileSymbolChunks(f.ID, model)
		if err != nil {
			return err
		}
		ids, err := ix.DB.SymbolIDsByFile(f.ID)
		if err != nil {
			return err
		}
		byIdentity := make(map[string]storedChunk, len(chunks))
		for _, c := range chunks {
			byIdentity[symbolIdentity(c.Name, c.Container, c.Signature)] = storedChunk{sha: c.SHA, vec: c.Vec}
		}
		caps[path] = fileCapture{oldFileID: f.ID, oldSymbolIDs: ids, byIdentity: byIdentity}
		return nil
	}

	for _, o := range outcomes {
		if o.parsed != nil {
			if err := capture(o.rel); err != nil {
				return nil, err
			}
		}
	}
	for path := range existing {
		if !candSet[path] {
			if err := capture(path); err != nil {
				return nil, err
			}
		}
	}
	return caps, nil
}

// vecSlot holds one symbol vector in file order, for the file-vector mean.
type vecSlot struct {
	vec []float32
	set bool
}

// embed runs after the metadata write: it deletes orphaned vectors, embeds the
// changed symbol/file chunks (carrying unchanged ones), and derives each
// touched file's vector as the mean of its symbol vectors.
func (ix *Indexer) embed(mode Mode, outcomes []outcome, caps map[string]fileCapture, existing map[string]string, candSet map[string]bool, res *IndexResult) error {
	model := ix.Embedder.Model()
	dim := ix.Embedder.Dim()

	// 1. Drop vectors orphaned by the write: changed files (ids churned) and,
	//    in incremental mode, deleted files.
	for _, o := range outcomes {
		if o.parsed != nil {
			if err := ix.dropCaptured(caps[o.rel]); err != nil {
				return err
			}
		}
	}
	if mode == ModeIncremental {
		for path := range existing {
			if !candSet[path] {
				if err := ix.dropCaptured(caps[path]); err != nil {
					return err
				}
			}
		}
	}

	// 2. Plan per-file work: align new symbol ids with parsed symbols, carrying
	//    unchanged vectors and queueing changed ones for a single batch embed.
	type symJob struct {
		fileID, symbolID int64
		slot             int
		text, sha        string
	}
	type fileJob struct {
		fileID int64
		sha    string
	}

	var symJobs []symJob
	var fileJobs []fileJob
	var fallbackTexts []string
	fileVecs := make(map[int64][]vecSlot)

	for _, o := range outcomes {
		p := o.parsed
		if p == nil {
			continue
		}
		f, ok, err := ix.DB.FileByPath(o.rel)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}

		// Files with no extracted symbols embed a compact fallback text.
		if len(p.symbols) == 0 {
			text := fileFallbackText(p)
			fallbackTexts = append(fallbackTexts, text)
			fileJobs = append(fileJobs, fileJob{fileID: f.ID, sha: chunkSHA(model, text)})
			continue
		}

		ids, err := ix.DB.SymbolIDsByFile(f.ID)
		if err != nil {
			return err
		}
		n := min(len(ids), len(p.exsyms))
		fc := caps[o.rel]
		slots := make([]vecSlot, n)
		for i := 0; i < n; i++ {
			text := chunkText(p.exsyms[i], p.content)
			sha := chunkSHA(model, text)
			id := ids[i]
			key := symbolIdentity(p.exsyms[i].Name, p.exsyms[i].Container, p.exsyms[i].Signature)
			if old, ok := fc.byIdentity[key]; ok && old.sha == sha && len(old.vec) == dim {
				// Unchanged: carry the stored vector to the new id, no inference.
				if err := ix.DB.UpsertVector(symbolOwner, id, model, sha, old.vec); err != nil {
					return err
				}
				slots[i] = vecSlot{vec: old.vec, set: true}
				res.EmbedSkipped++
				continue
			}
			symJobs = append(symJobs, symJob{fileID: f.ID, symbolID: id, slot: i, text: text, sha: sha})
		}
		fileVecs[f.ID] = slots
	}

	// 3. One batch embed for every changed symbol chunk + fallback file chunk.
	texts := make([]string, 0, len(symJobs)+len(fallbackTexts))
	for _, j := range symJobs {
		texts = append(texts, j.text)
	}
	texts = append(texts, fallbackTexts...)

	var vecs [][]float32
	if len(texts) > 0 {
		v, err := ix.Embedder.Embed(texts)
		if err != nil {
			return embedErr(err)
		}
		vecs = v
	}

	// 4. Store symbol vectors and slot them for the file mean.
	for i, j := range symJobs {
		v := vecs[i]
		if err := ix.DB.UpsertVector(symbolOwner, j.symbolID, model, j.sha, v); err != nil {
			return err
		}
		res.Embedded++
		fileVecs[j.fileID][j.slot] = vecSlot{vec: v, set: true}
	}
	// 5. Store fallback file vectors (no symbols -> direct embed).
	for k, fj := range fileJobs {
		v := vecs[len(symJobs)+k]
		if err := ix.DB.UpsertVector(fileOwner, fj.fileID, model, fj.sha, v); err != nil {
			return err
		}
		res.Embedded++
	}

	// 6. Derive each symbol-bearing file's vector as the normalized mean.
	for fileID, slots := range fileVecs {
		mean := meanNormalize(slots, dim)
		if mean == nil {
			continue
		}
		if err := ix.DB.UpsertVector(fileOwner, fileID, model, vecSHA(model, mean), mean); err != nil {
			return err
		}
	}

	// 7. Embed memories so the durable record is semantically searchable.
	if err := ix.embedMemories(model, res); err != nil {
		return err
	}

	return ix.DB.Meta().SetEmbedInfo(model, dim)
}

// embedMemories re-embeds the memories whose text changed (content_sha gate)
// under the current model. Deletions are handled at delete time (store drops
// the owner's vector), so this only adds/updates.
func (ix *Indexer) embedMemories(model string, res *IndexResult) error {
	type job struct {
		id   int64
		sha  string
		text string
	}
	ids, err := ix.DB.AllMemoryIDs()
	if err != nil {
		return err
	}
	var jobs []job
	for _, id := range ids {
		m, ok, err := ix.DB.MemoryFull(id)
		if err != nil {
			return err
		}
		text := memoryText(m.Title, m.Body, m.Tags)
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		sha := chunkSHA(model, text)
		prev, exists, err := ix.DB.ChunkSHA("memory", id, model)
		if err != nil {
			return err
		}
		if exists && prev == sha {
			res.EmbedSkipped++
			continue
		}
		jobs = append(jobs, job{id, sha, text})
	}
	if len(jobs) == 0 {
		return nil
	}

	texts := make([]string, len(jobs))
	for i, j := range jobs {
		texts[i] = j.text
	}
	vecs, err := ix.Embedder.Embed(texts)
	if err != nil {
		return embedErr(err)
	}
	for i, j := range jobs {
		if err := ix.DB.UpsertVector("memory", j.id, model, j.sha, vecs[i]); err != nil {
			return err
		}
		res.Embedded++
	}
	return nil
}

// memoryText is the embed text for a memory: title, body and tags joined.
func memoryText(title, body string, tags []string) string {
	parts := make([]string, 0, 3)
	if title != "" {
		parts = append(parts, title)
	}
	if body != "" {
		parts = append(parts, body)
	}
	if len(tags) > 0 {
		parts = append(parts, strings.Join(tags, " "))
	}
	return strings.Join(parts, "\n")
}

// dropCaptured removes the captured file's stale symbol and file vectors.
func (ix *Indexer) dropCaptured(c fileCapture) error {
	if c.oldFileID == 0 {
		return nil
	}
	if len(c.oldSymbolIDs) > 0 {
		if err := ix.DB.DeleteVectors(symbolOwner, c.oldSymbolIDs); err != nil {
			return err
		}
	}
	return ix.DB.DeleteVectors(fileOwner, []int64{c.oldFileID})
}

// chunkText builds the text embedded for a symbol: a compact, identifier-rich
// rendering — container, name, signature, then the body source from the AST
// span (read live from src, never stored).
func chunkText(s extract.Symbol, src []byte) string {
	var b strings.Builder
	if s.Container != "" {
		b.WriteString(s.Container)
		b.WriteByte(' ')
	}
	b.WriteString(s.Name)
	if s.Signature != "" {
		b.WriteByte('\n')
		b.WriteString(s.Signature)
	}
	if body := bodySpan(src, s.StartLine, s.EndLine); body != "" {
		b.WriteByte('\n')
		b.WriteString(body)
	}
	return b.String()
}

// bodySpan returns the source for the inclusive 1-based line range, or "" when
// out of bounds. src may be nil (non-parsed file).
func bodySpan(src []byte, startLine, endLine int) string {
	if len(src) == 0 || startLine <= 0 || endLine < startLine {
		return ""
	}
	lines := strings.Split(string(src), "\n")
	if startLine > len(lines) {
		return ""
	}
	end := endLine
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[startLine-1:end], "\n")
}

// fileFallbackText is the embed text for a file with no extracted symbols:
// path, package, role, any top-level signatures, and — for non-code files — the
// file's content (head-sliced) so manifests, docs, build and CI files are
// searchable by what they contain, not just where they live.
func fileFallbackText(p *parsed) string {
	parts := make([]string, 0, 4+len(p.symbols))
	for _, s := range []string{p.record.Path, p.record.Package, p.record.Role} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	for _, s := range p.symbols {
		if s.Signature != "" {
			parts = append(parts, s.Signature)
		}
	}
	if len(p.content) > 0 {
		parts = append(parts, string(headSlice(p.content)))
	}
	return strings.Join(parts, " ")
}

// symbolIdentity keys a symbol by its stable content identity (independent of
// the churning row id) so a vector can be carried across a reindex.
func symbolIdentity(name, container, signature string) string {
	return container + "\x00" + name + "\x00" + signature
}

// chunkSHA gates re-embedding: model is mixed in so a model change invalidates
// every chunk.
func chunkSHA(model, text string) string {
	h := sha256.Sum256([]byte(model + "\x00" + text))
	return hex.EncodeToString(h[:])
}

// vecSHA derives a content_sha for a derived (mean) file vector so it is
// rewritten exactly when the vector changes.
func vecSHA(model string, v []float32) string {
	h := sha256.New()
	h.Write([]byte(model))
	var b [4]byte
	for _, x := range v {
		binary.LittleEndian.PutUint32(b[:], math.Float32bits(x))
		h.Write(b[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// meanNormalize averages the set slots and L2-normalizes, returning nil when no
// slot is populated.
func meanNormalize(slots []vecSlot, dim int) []float32 {
	sum := make([]float64, dim)
	n := 0
	for _, s := range slots {
		if !s.set || len(s.vec) != dim {
			continue
		}
		for i, x := range s.vec {
			sum[i] += float64(x)
		}
		n++
	}
	if n == 0 {
		return nil
	}
	out := make([]float32, dim)
	var norm float64
	for i := range sum {
		avg := sum[i] / float64(n)
		out[i] = float32(avg)
		norm += avg * avg
	}
	if norm > 0 {
		inv := float32(1.0 / math.Sqrt(norm))
		for i := range out {
			out[i] *= inv
		}
	}
	return out
}
