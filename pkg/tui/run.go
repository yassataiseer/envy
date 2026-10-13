package tui

import (
	"io"

	tea "charm.land/bubbletea/v2"
)

// Run starts the interactive diff view and blocks until the user quits.
func Run(cfg Config, in io.Reader, out io.Writer) (Model, error) {
	p := tea.NewProgram(New(cfg), tea.WithInput(in), tea.WithOutput(out))
	final, err := p.Run()
	if err != nil {
		return Model{}, err
	}
	return final.(Model), nil
}

// HasDrift reports whether the files still differ when the session ended.
func (m Model) HasDrift() bool {
	return m.set.HasDrift
}

// Unsaved reports whether the user quit with unsaved changes.
func (m Model) Unsaved() bool {
	return m.dirty[sideLeft] || m.dirty[sideRight]
}
