package tui

import (
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the interactive dashboard against the given Source and blocks until
// the user quits.
func Run(src Source, opts ...Option) error {
	_, err := tea.NewProgram(New(src, opts...), tea.WithAltScreen()).Run()
	return err
}

// PrintFrame renders a single dashboard frame at a fixed size to w, without a
// terminal. It is used for headless verification and snapshot-style debugging.
func PrintFrame(w io.Writer, src Source, width, height int) error {
	snap, err := src.Load()
	if err != nil {
		return err
	}
	m := New(src)
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	next, _ = next.(Model).Update(snapshotMsg{snap: snap})
	_, err = fmt.Fprintln(w, next.(Model).View())
	return err
}
