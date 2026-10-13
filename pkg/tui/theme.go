package tui

import (
	"charm.land/lipgloss/v2"
	"github.com/yassataiseer/envy/pkg/diff"
)

type theme struct {
	Title        lipgloss.Style
	TitleFocused lipgloss.Style
	Rule         lipgloss.Style
	Separator    lipgloss.Style

	Status map[diff.DiffStatus]lipgloss.Style
	Value  lipgloss.Style
	Masked lipgloss.Style
	Absent lipgloss.Style

	SelectedFocused lipgloss.Style
	Selected        lipgloss.Style

	Bar      lipgloss.Style
	BarError lipgloss.Style
	BarWarn  lipgloss.Style
	BarInfo  lipgloss.Style
	Dim      lipgloss.Style
}

// newTheme builds styles from the 16 basic ANSI colors so they follow the
// user's terminal palette. High-contrast mode drops faint text and uses
// bright, bold colors.
func newTheme(isDark, highContrast bool) theme {
	ld := lipgloss.LightDark(isDark)
	red, green, yellow, magenta, blue := lipgloss.Color("1"), lipgloss.Color("2"), lipgloss.Color("3"), lipgloss.Color("5"), lipgloss.Color("4")
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	subtleBg := ld(lipgloss.Color("254"), lipgloss.Color("237"))
	if highContrast {
		red, green, yellow, magenta, blue = lipgloss.Color("9"), lipgloss.Color("10"), lipgloss.Color("11"), lipgloss.Color("13"), lipgloss.Color("12")
		dim = lipgloss.NewStyle()
		subtleBg = ld(lipgloss.Color("250"), lipgloss.Color("240"))
	}
	bold := func(s lipgloss.Style) lipgloss.Style { return s.Bold(highContrast) }

	return theme{
		Title:        dim.Bold(true),
		TitleFocused: lipgloss.NewStyle().Bold(true).Foreground(blue).Underline(true),
		Rule:         dim,
		Separator:    dim,

		Status: map[diff.DiffStatus]lipgloss.Style{
			diff.StatusIdentical:    lipgloss.NewStyle(),
			diff.StatusValueDiff:    bold(lipgloss.NewStyle().Foreground(yellow)),
			diff.StatusTypeMismatch: bold(lipgloss.NewStyle().Foreground(magenta)),
			diff.StatusMissingLeft:  bold(lipgloss.NewStyle().Foreground(green)),
			diff.StatusMissingRight: bold(lipgloss.NewStyle().Foreground(red)),
		},
		Value:  lipgloss.NewStyle(),
		Masked: dim,
		Absent: dim.Italic(true),

		SelectedFocused: lipgloss.NewStyle().Reverse(true).Bold(true),
		Selected:        lipgloss.NewStyle().Background(subtleBg),

		Bar:      dim,
		BarError: lipgloss.NewStyle().Foreground(red).Bold(true),
		BarWarn:  lipgloss.NewStyle().Foreground(yellow).Bold(true),
		BarInfo:  lipgloss.NewStyle().Foreground(green),
		Dim:      dim,
	}
}
