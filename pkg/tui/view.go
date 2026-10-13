package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/yassataiseer/envy/pkg/diff"
	"github.com/yassataiseer/envy/pkg/envparse"
)

const (
	minWidth     = 40
	minHeight    = 8
	headerHeight = 2
	maskText     = "••••••••"
	absentText   = "MISSING KEY"
)

func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "envy diff"
	return v
}

// minBodyRows is the fewest diff rows to keep visible; the full help view
// falls back to the one-line version when it would leave fewer.
const minBodyRows = 3

// helpView renders the help footer clipped to the window width. The help
// bubble can overrun its width when an item lands exactly at the edge.
func (m Model) helpView() string {
	h := m.help
	if h.ShowAll && m.height-headerHeight-1-lipgloss.Height(h.View(m.keys)) < minBodyRows {
		h.ShowAll = false
	}
	lines := strings.Split(h.View(m.keys), "\n")
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, m.width, "…")
	}
	return strings.Join(lines, "\n")
}

func (m Model) footerHeight() int {
	return 1 + lipgloss.Height(m.helpView())
}

func (m Model) bodyHeight() int {
	return max(0, m.height-headerHeight-m.footerHeight())
}

func (m Model) paneWidths() (int, int) {
	left := (m.width - 1) / 2
	return left, m.width - 1 - left
}

func (m Model) render() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if m.width < minWidth || m.height < minHeight {
		return fmt.Sprintf("Terminal too small (%dx%d); envy needs at least %dx%d.", m.width, m.height, minWidth, minHeight)
	}

	lines := make([]string, 0, m.height)
	lines = append(lines, m.renderHeader()...)
	lines = append(lines, m.renderBody()...)
	lines = append(lines, m.renderStatus())
	lines = append(lines, m.helpView())
	return strings.Join(lines, "\n")
}

func (m Model) renderHeader() []string {
	lw, rw := m.paneWidths()
	title := func(s side, w int) string {
		suffix := ""
		if m.dirty[s] {
			suffix = " (modified)"
		}
		path := truncateLeft(m.name(s), w-1-ansi.StringWidth(suffix))
		style := m.theme.Title
		if s == m.focus {
			style = m.theme.TitleFocused
		}
		return pad(style.Render(ansi.Truncate(" "+path+suffix, w, "…")), w)
	}
	sep := m.theme.Separator.Render("│")
	rule := m.theme.Rule.Render(strings.Repeat("─", lw) + "┼" + strings.Repeat("─", rw))
	return []string{title(sideLeft, lw) + sep + title(sideRight, rw), rule}
}

func (m Model) renderBody() []string {
	h := m.bodyHeight()
	lw, rw := m.paneWidths()
	lines := make([]string, 0, h)

	if len(m.rows) == 0 {
		msg := "Both files are empty."
		if m.onlyDiffs && len(m.set.Results) > 0 {
			msg = "No differences. Press f to show all keys."
		}
		for i := range h {
			if i == h/2 {
				lines = append(lines, lipgloss.PlaceHorizontal(m.width, lipgloss.Center, m.theme.Dim.Render(msg)))
			} else {
				lines = append(lines, "")
			}
		}
		return lines
	}

	sep := m.theme.Separator.Render("│")
	for i := m.offset; i < m.offset+h; i++ {
		if i >= len(m.rows) {
			lines = append(lines, strings.Repeat(" ", lw)+sep)
			continue
		}
		r := m.set.Results[m.rows[i]]
		selected := i == m.cursor
		lines = append(lines, m.renderCell(r, sideLeft, lw, selected)+sep+m.renderCell(r, sideRight, rw, selected))
	}
	return lines
}

// renderCell renders one pane's half of a row: a status gutter, then
// KEY=value or a MISSING KEY placeholder, truncated to width.
func (m Model) renderCell(r diff.DiffResult, s side, width int, selected bool) string {
	gutter := " " + r.Status.Marker() + " "
	if s == sideRight {
		gutter = "  "
	}
	avail := width - ansi.StringWidth(gutter)

	e := entryFor(r, s)
	var keyText, valueText string
	switch {
	case e == nil:
		keyText = absentText
	case e.IsBareExport:
		keyText = "export " + e.Key
	default:
		keyText = e.Key
		valueText = "=" + m.displayValue(e)
	}
	keyText = ansi.Truncate(keyText, avail, "…")
	valueText = ansi.Truncate(valueText, avail-ansi.StringWidth(keyText), "…")

	if selected {
		style := m.theme.Selected
		if s == m.focus {
			style = m.theme.SelectedFocused
		}
		return style.Render(pad(gutter+keyText+valueText, width))
	}

	statusStyle := m.theme.Status[r.Status]
	var b strings.Builder
	b.WriteString(statusStyle.Render(gutter))
	switch {
	case e == nil:
		b.WriteString(m.theme.Absent.Render(keyText))
	default:
		b.WriteString(statusStyle.Render(keyText))
		valueStyle := m.theme.Value
		if m.masked && e.Value != "" {
			valueStyle = m.theme.Masked
		}
		b.WriteString(valueStyle.Render(valueText))
	}
	return pad(b.String(), width)
}

// displayValue returns a single-line, terminal-safe rendering of a value.
// Masked values use a fixed-width placeholder so their length is not
// revealed.
func (m Model) displayValue(e *envparse.EnvEntry) string {
	if e.Value == "" {
		return ""
	}
	if m.masked {
		return maskText
	}
	return sanitize(e.Value)
}

// sanitize replaces control characters so values cannot inject terminal
// escape sequences or break the row layout.
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n':
			return '⏎'
		case r == '\t':
			return ' '
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0):
			return '·'
		}
		return r
	}, s)
}

// renderStatus shows a transient message or the diff summary on the left,
// and the mask/filter flags on the right so that revealed values are
// always flagged.
func (m Model) renderStatus() string {
	left := m.renderMessage()
	if left == "" {
		left = m.renderSummary()
	}

	var flags []string
	if m.onlyDiffs {
		flags = append(flags, "diffs only")
	}
	if m.masked {
		flags = append(flags, "masked")
	} else {
		flags = append(flags, "REVEALED")
	}
	rightStyle := m.theme.Bar
	if !m.masked {
		rightStyle = m.theme.BarWarn
	}
	right := rightStyle.Render("[" + strings.Join(flags, "] [") + "] ")

	rightW := lipgloss.Width(right)
	if rightW >= m.width {
		return ansi.Truncate(left, m.width, "…")
	}
	left = ansi.Truncate(left, m.width-rightW-1, "…")
	return left + strings.Repeat(" ", m.width-lipgloss.Width(left)-rightW) + right
}

func (m Model) renderMessage() string {
	if m.message == "" {
		return ""
	}
	style := m.theme.BarInfo
	switch m.messageKind {
	case msgWarn:
		style = m.theme.BarWarn
	case msgError:
		style = m.theme.BarError
	}
	return style.Render(" " + m.message)
}

func (m Model) renderSummary() string {
	sum := m.set.Summary()
	var parts []string
	if sum.Differences() == 0 {
		parts = append(parts, m.theme.BarInfo.Render(fmt.Sprintf("%d keys, no differences", len(m.set.Results))))
	} else {
		parts = append(parts, m.theme.Bar.Render(fmt.Sprintf("%d keys", len(m.set.Results))))
		add := func(n int, status diff.DiffStatus, label string) {
			if n > 0 {
				parts = append(parts, m.theme.Status[status].Render(fmt.Sprintf("%s %d %s", status.Marker(), n, label)))
			}
		}
		add(sum.ValueDiff, diff.StatusValueDiff, "changed")
		add(sum.TypeMismatch, diff.StatusTypeMismatch, "type mismatch")
		add(sum.MissingRight, diff.StatusMissingRight, "missing in "+m.shortName(sideRight))
		add(sum.MissingLeft, diff.StatusMissingLeft, "missing in "+m.shortName(sideLeft))
	}
	return " " + strings.Join(parts, m.theme.Bar.Render(" · "))
}

// truncateLeft shortens s to width cells by dropping its beginning, which
// keeps the file name visible for long paths.
func truncateLeft(s string, width int) string {
	w := ansi.StringWidth(s)
	if w <= width {
		return s
	}
	if width < 1 {
		return ""
	}
	return ansi.TruncateLeft(s, w-width+1, "…")
}

// pad right-pads s with spaces to exactly width display cells.
func pad(s string, width int) string {
	if w := lipgloss.Width(s); w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}
