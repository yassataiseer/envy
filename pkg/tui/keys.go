package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Up          key.Binding
	Down        key.Binding
	PageUp      key.Binding
	PageDown    key.Binding
	Top         key.Binding
	Bottom      key.Binding
	FocusLeft   key.Binding
	FocusRight  key.Binding
	SwitchFocus key.Binding
	CopyFocused key.Binding
	CopyToRight key.Binding
	CopyToLeft  key.Binding
	Delete      key.Binding
	Mask        key.Binding
	Filter      key.Binding
	Undo        key.Binding
	Save        key.Binding
	Help        key.Binding
	Quit        key.Binding
	ForceQuit   key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Up:          key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:        key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		PageUp:      key.NewBinding(key.WithKeys("pgup", "ctrl+u"), key.WithHelp("pgup", "page up")),
		PageDown:    key.NewBinding(key.WithKeys("pgdown", "ctrl+d"), key.WithHelp("pgdn", "page down")),
		Top:         key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g", "top")),
		Bottom:      key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G", "bottom")),
		FocusLeft:   key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "focus left")),
		FocusRight:  key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "focus right")),
		SwitchFocus: key.NewBinding(key.WithKeys("tab", "shift+tab"), key.WithHelp("tab", "switch pane")),
		CopyFocused: key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "copy to other pane")),
		CopyToRight: key.NewBinding(key.WithKeys(">"), key.WithHelp(">", "copy left → right")),
		CopyToLeft:  key.NewBinding(key.WithKeys("<"), key.WithHelp("<", "copy right → left")),
		Delete:      key.NewBinding(key.WithKeys("d", "delete"), key.WithHelp("d", "delete from pane")),
		Mask:        key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "mask values")),
		Filter:      key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "only differences")),
		Undo:        key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "undo")),
		Save:        key.NewBinding(key.WithKeys("s", "ctrl+s"), key.WithHelp("s", "save")),
		Help:        key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more keys")),
		Quit:        key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "quit")),
		ForceQuit:   key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit without saving")),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Down, k.SwitchFocus, k.CopyFocused, k.Mask, k.Filter, k.Save, k.Quit, k.Help}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.Top, k.Bottom},
		{k.SwitchFocus, k.FocusLeft, k.FocusRight},
		{k.CopyFocused, k.CopyToRight, k.CopyToLeft, k.Delete, k.Undo},
		{k.Mask, k.Filter, k.Save, k.Quit, k.ForceQuit, k.Help},
	}
}
