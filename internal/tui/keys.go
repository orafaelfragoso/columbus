package tui

import "charm.land/bubbles/v2/key"

// keyMap is the dashboard's global keybindings. It implements help.KeyMap so the
// footer help bar is generated from the same source of truth that handles input.
type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Left     key.Binding
	Right    key.Binding
	WorkPrev key.Binding
	WorkNext key.Binding
	Tab      key.Binding
	ShiftTab key.Binding
	Enter    key.Binding
	Search   key.Binding
	Refresh  key.Binding
	Reindex  key.Binding
	Esc      key.Binding
	Help     key.Binding
	Quit     key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Left:     key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "prev column")),
		Right:    key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next column")),
		WorkPrev: key.NewBinding(key.WithKeys("["), key.WithHelp("[", "prev type")),
		WorkNext: key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next type")),
		Tab:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next view")),
		ShiftTab: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev view")),
		Enter:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "detail")),
		Search:   key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Refresh:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Reindex:  key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "reindex")),
		Esc:      key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Tab, k.Up, k.Down, k.Left, k.Right, k.WorkPrev, k.WorkNext, k.Enter, k.Search, k.Refresh, k.Reindex, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right, k.Tab, k.ShiftTab, k.Enter},
		{k.Search, k.Refresh, k.Reindex, k.Esc},
		{k.Help, k.Quit},
	}
}

// staticHelp is a fixed help.KeyMap used to give each modal (search, results,
// detail) its own footer hints instead of the dashboard's tab-navigation keys,
// which don't apply while a modal owns the input.
type staticHelp struct{ keys []key.Binding }

func (s staticHelp) ShortHelp() []key.Binding  { return s.keys }
func (s staticHelp) FullHelp() [][]key.Binding { return [][]key.Binding{s.keys} }

func searchHelpKeys() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "search")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	}
}

func resultsHelpKeys() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "select")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}

func detailHelpKeys() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "scroll")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}

func quitHelpKeys() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("y", "enter"), key.WithHelp("y/enter", "confirm")),
		key.NewBinding(key.WithKeys("n", "esc"), key.WithHelp("n/esc", "cancel")),
	}
}

func mainHelpKeys(k keyMap) []key.Binding {
	return []key.Binding{k.Tab, k.Up, k.Down, k.Enter, k.Search, k.Refresh, k.Reindex, k.Help, k.Quit}
}

func workHelpKeys(k keyMap) []key.Binding {
	return []key.Binding{k.Tab, k.WorkPrev, k.WorkNext, k.Left, k.Right, k.Up, k.Down, k.Enter, k.Search, k.Refresh, k.Reindex, k.Help, k.Quit}
}
