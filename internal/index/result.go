package index

import (
	"fmt"
	"io"

	"github.com/orafaelfragoso/columbus/internal/render"
)

// IndexResult is the typed result of an index operation.
type IndexResult struct {
	Mode              string   `json:"mode"`
	Indexed           int      `json:"indexed"`
	Unchanged         int      `json:"unchanged"`
	Deleted           int      `json:"deleted"`
	SkippedBinary     int      `json:"skipped_binary"`
	SkippedOversized  int      `json:"skipped_oversized"`
	SkippedUnreadable int      `json:"skipped_unreadable"`
	Symbols           int      `json:"symbols"`
	Embedded          int      `json:"embedded,omitempty"`      // chunks sent through the embedder
	EmbedSkipped      int      `json:"embed_skipped,omitempty"` // unchanged chunks carried forward
	TotalFiles        int      `json:"total_files"`
	IndexedHead       string   `json:"indexed_head"`
	Dirty             bool     `json:"dirty"`
	StatusOnly        bool     `json:"status_only,omitempty"`
	Warnings          []string `json:"warnings,omitempty"`
}

func (IndexResult) CommandName() string { return "index" }

func (r IndexResult) RenderText(w io.Writer, _ render.Options) error {
	verb := "Indexed"
	if r.StatusOnly {
		verb = "Would index"
	}
	if r.Mode == "clean" {
		fmt.Fprintf(w, "Cleaned index (%d files removed)\n", r.Deleted)
		return nil
	}
	fmt.Fprintf(w, "%s %d file(s) [%s mode]\n", verb, r.Indexed, r.Mode)
	fmt.Fprintf(w, "  unchanged: %d  deleted: %d  symbols: %d\n", r.Unchanged, r.Deleted, r.Symbols)
	if r.Embedded > 0 || r.EmbedSkipped > 0 {
		fmt.Fprintf(w, "  embedded:  %d  skipped (unchanged): %d\n", r.Embedded, r.EmbedSkipped)
	}
	if skipped := r.SkippedBinary + r.SkippedOversized + r.SkippedUnreadable; skipped > 0 {
		fmt.Fprintf(w, "  skipped:   %d (binary %d, oversized %d, unreadable %d)\n",
			skipped, r.SkippedBinary, r.SkippedOversized, r.SkippedUnreadable)
	}
	fmt.Fprintf(w, "  index now: %d files\n", r.TotalFiles)
	if r.Dirty {
		fmt.Fprintln(w, "  working tree: dirty")
	}
	for _, warn := range r.Warnings {
		fmt.Fprintf(w, "  warning: %s\n", warn)
	}
	return nil
}

func (r IndexResult) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# columbus index (%s)\n\n", r.Mode)
	fmt.Fprintf(w, "- indexed: %d\n- unchanged: %d\n- deleted: %d\n- symbols: %d\n- total_files: %d\n- dirty: %t\n",
		r.Indexed, r.Unchanged, r.Deleted, r.Symbols, r.TotalFiles, r.Dirty)
	return nil
}
