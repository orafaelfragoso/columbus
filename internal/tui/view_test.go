package tui

import (
	"strings"
	"testing"
)

func TestMemColumnsIncludesTagsWhenWide(t *testing.T) {
	cols := memColumns(memTagsMinInner)
	if len(cols) != 3 {
		t.Fatalf("columns = %d, want 3 (Kind, Title, Tags)", len(cols))
	}
	if cols[2].Title != "Tags" || cols[2].Width != memTagsW {
		t.Errorf("tags column = %q/%d, want Tags/%d", cols[2].Title, cols[2].Width, memTagsW)
	}
	// Title is squeezed to leave room for Kind + Tags.
	if want := colW(memTagsMinInner - memKindW - memTagsW); cols[1].Width != want {
		t.Errorf("title width = %d, want %d", cols[1].Width, want)
	}
}

func TestMemColumnsDropsTagsWhenNarrow(t *testing.T) {
	cols := memColumns(memTagsMinInner - 1)
	if len(cols) != 2 {
		t.Fatalf("columns = %d, want 2 (Tags dropped on narrow pane)", len(cols))
	}
	if cols[1].Title != "Title" {
		t.Errorf("second column = %q, want Title", cols[1].Title)
	}
}

func TestMemTableRowsRendersAndTruncatesTags(t *testing.T) {
	mems := []MemRow{
		{ID: "mem_001", Kind: "adr", Title: "use WAL", Tags: []string{"db", "storage"}},
		{ID: "mem_002", Kind: "plan", Title: "race", Tags: []string{"averyverylongtagname", "second", "third"}},
	}

	wide := memTableRows(mems, memTagsMinInner)
	if len(wide[0]) != 3 {
		t.Fatalf("wide row cells = %d, want 3", len(wide[0]))
	}
	if wide[0][2] != "db,storage" {
		t.Errorf("tags cell = %q, want db,storage", wide[0][2])
	}
	// Overflowing tags are truncated with an ellipsis to the column width.
	if got := wide[1][2]; !strings.HasSuffix(got, "…") {
		t.Errorf("overflowing tags = %q, want ellipsis suffix", got)
	}

	narrow := memTableRows(mems, memTagsMinInner-1)
	if len(narrow[0]) != 2 {
		t.Errorf("narrow row cells = %d, want 2 (no tags cell)", len(narrow[0]))
	}
}
