// Package tui implements the interactive side-by-side diff view.
package tui

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/yassataiseer/envy/pkg/audit"
	"github.com/yassataiseer/envy/pkg/diff"
	"github.com/yassataiseer/envy/pkg/envparse"
)

// Config configures a diff session.
type Config struct {
	Left, Right  *envparse.EnvFile
	Options      diff.Options
	HighContrast bool
	// Reveal starts with values visible instead of masked.
	Reveal bool
}

type side int

const (
	sideLeft side = iota
	sideRight
)

func (s side) other() side { return 1 - s }

type messageKind int

const (
	msgInfo messageKind = iota
	msgWarn
	msgError
)

const maxUndo = 100

type snapshot struct {
	side    side
	content []byte
}

// Model is the Bubble Tea model for the diff view.
type Model struct {
	files [2]*envparse.EnvFile
	saved [2][]byte
	dirty [2]bool
	opts  diff.Options

	set *diff.FileDiffSet
	// findings holds the most severe security finding per key in each file.
	findings [2]map[string]audit.AuditFinding
	rows     []int // indices into set.Results that are currently visible
	cursor   int   // index into rows
	offset   int   // first visible row

	width, height int
	focus         side
	masked        bool
	onlyDiffs     bool
	confirmQuit   bool
	undo          []snapshot

	message     string
	messageKind messageKind

	keys         keyMap
	help         help.Model
	theme        theme
	isDark       bool
	highContrast bool
}

// New creates a model comparing cfg.Left (source) and cfg.Right (target).
func New(cfg Config) Model {
	m := Model{
		files:        [2]*envparse.EnvFile{cfg.Left, cfg.Right},
		opts:         cfg.Options,
		masked:       !cfg.Reveal,
		keys:         defaultKeyMap(),
		help:         help.New(),
		isDark:       true,
		highContrast: cfg.HighContrast,
	}
	for s := range m.files {
		m.saved[s] = m.files[s].Format()
	}
	m.applyTheme()
	m.recompute("")
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

func (m *Model) applyTheme() {
	m.theme = newTheme(m.isDark, m.highContrast)
	m.help.Styles = help.DefaultStyles(m.isDark)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
		m.clampScroll()
		return m, nil
	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		m.applyTheme()
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.ForceQuit) {
		return m, tea.Quit
	}
	if m.confirmQuit {
		m.confirmQuit = false
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Save):
			if m.save() {
				return m, tea.Quit
			}
			return m, nil
		}
		m.setMessage(msgInfo, "Quit cancelled.")
		return m, nil
	}
	m.message = ""

	switch {
	case key.Matches(msg, m.keys.Quit):
		if m.dirty[sideLeft] || m.dirty[sideRight] {
			m.confirmQuit = true
			m.setMessage(msgWarn, "Unsaved changes. Press q again to discard, s to save and quit, any other key to cancel.")
			return m, nil
		}
		return m, tea.Quit
	case key.Matches(msg, m.keys.Up):
		m.moveCursor(-1)
	case key.Matches(msg, m.keys.Down):
		m.moveCursor(1)
	case key.Matches(msg, m.keys.PageUp):
		m.moveCursor(-m.bodyHeight())
	case key.Matches(msg, m.keys.PageDown):
		m.moveCursor(m.bodyHeight())
	case key.Matches(msg, m.keys.Top):
		m.moveCursor(-len(m.rows))
	case key.Matches(msg, m.keys.Bottom):
		m.moveCursor(len(m.rows))
	case key.Matches(msg, m.keys.FocusLeft):
		m.focus = sideLeft
	case key.Matches(msg, m.keys.FocusRight):
		m.focus = sideRight
	case key.Matches(msg, m.keys.SwitchFocus):
		m.focus = m.focus.other()
	case key.Matches(msg, m.keys.CopyFocused):
		m.copySelected(m.focus)
	case key.Matches(msg, m.keys.CopyToRight):
		m.copySelected(sideLeft)
	case key.Matches(msg, m.keys.CopyToLeft):
		m.copySelected(sideRight)
	case key.Matches(msg, m.keys.Delete):
		m.deleteSelected()
	case key.Matches(msg, m.keys.Undo):
		m.undoLast()
	case key.Matches(msg, m.keys.Mask):
		m.masked = !m.masked
		if m.masked {
			m.setMessage(msgInfo, "Values masked.")
		} else {
			m.setMessage(msgWarn, "Values revealed. Press m to mask again.")
		}
	case key.Matches(msg, m.keys.Filter):
		m.onlyDiffs = !m.onlyDiffs
		m.recompute(m.selectedKey())
	case key.Matches(msg, m.keys.Save):
		m.save()
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		m.clampScroll()
	}
	return m, nil
}

func (m *Model) setMessage(kind messageKind, format string, args ...any) {
	m.messageKind = kind
	m.message = fmt.Sprintf(format, args...)
}

func (m *Model) selected() (diff.DiffResult, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return diff.DiffResult{}, false
	}
	return m.set.Results[m.rows[m.cursor]], true
}

func (m *Model) selectedKey() string {
	r, _ := m.selected()
	return r.Key
}

func entryFor(r diff.DiffResult, s side) *envparse.EnvEntry {
	if s == sideLeft {
		return r.LeftEntry
	}
	return r.RightEntry
}

func (m *Model) name(s side) string {
	if p := m.files[s].Path; p != "" {
		return p
	}
	return [2]string{"left", "right"}[s]
}

// shortName is the file's base name, or its full path when both files
// share a base name.
func (m *Model) shortName(s side) string {
	base := filepath.Base(m.name(s))
	if base == filepath.Base(m.name(s.other())) {
		return m.name(s)
	}
	return base
}

// copySelected copies the selected key's value from src to the other pane.
func (m *Model) copySelected(src side) {
	r, ok := m.selected()
	if !ok {
		return
	}
	from := entryFor(r, src)
	dst := src.other()
	switch {
	case from == nil:
		m.setMessage(msgWarn, "%s is not defined in %s; nothing to copy.", r.Key, m.shortName(src))
		return
	case r.Status == diff.StatusIdentical && entryFor(r, dst) != nil:
		m.setMessage(msgInfo, "%s is already identical.", r.Key)
		return
	}
	m.pushUndo(dst)
	if err := m.files[dst].SetFromAfter(from, m.anchorFor(r.Key, src, dst)); err != nil {
		m.undo = m.undo[:len(m.undo)-1]
		m.setMessage(msgError, "Copy failed: %v", err)
		return
	}
	m.afterEdit(dst, r.Key)
	m.setMessage(msgInfo, "Copied %s to %s.", r.Key, m.shortName(dst))
}

// anchorFor returns the nearest key above key in the src file that dst
// also defines, so a copied key lands next to the same neighbor.
func (m *Model) anchorFor(key string, src, dst side) string {
	anchor := ""
	for _, k := range m.files[src].Keys() {
		if k == key {
			break
		}
		if _, ok := m.files[dst].Get(k); ok {
			anchor = k
		}
	}
	return anchor
}

func (m *Model) deleteSelected() {
	r, ok := m.selected()
	if !ok {
		return
	}
	if entryFor(r, m.focus) == nil {
		m.setMessage(msgWarn, "%s is not defined in %s.", r.Key, m.shortName(m.focus))
		return
	}
	m.pushUndo(m.focus)
	m.files[m.focus].Delete(r.Key)
	m.afterEdit(m.focus, r.Key)
	m.setMessage(msgInfo, "Deleted %s from %s.", r.Key, m.shortName(m.focus))
}

func (m *Model) pushUndo(s side) {
	m.undo = append(m.undo, snapshot{side: s, content: m.files[s].Format()})
	if len(m.undo) > maxUndo {
		m.undo = m.undo[len(m.undo)-maxUndo:]
	}
}

func (m *Model) undoLast() {
	if len(m.undo) == 0 {
		m.setMessage(msgInfo, "Nothing to undo.")
		return
	}
	snap := m.undo[len(m.undo)-1]
	m.undo = m.undo[:len(m.undo)-1]
	restored, err := envparse.ParseBytes(snap.content, m.files[snap.side].Path)
	if err != nil {
		m.setMessage(msgError, "Undo failed: %v", err)
		return
	}
	key := m.selectedKey()
	m.files[snap.side] = restored
	m.afterEdit(snap.side, key)
	m.setMessage(msgInfo, "Undid last change to %s.", m.shortName(snap.side))
}

func (m *Model) afterEdit(s side, keepKey string) {
	m.dirty[s] = !bytes.Equal(m.files[s].Format(), m.saved[s])
	m.recompute(keepKey)
}

// save writes every modified file and reports whether all writes succeeded.
func (m *Model) save() bool {
	var written []string
	for s := range m.files {
		if !m.dirty[s] {
			continue
		}
		f := m.files[s]
		if err := f.WriteFile(f.Path); err != nil {
			m.recompute(m.selectedKey())
			m.setMessage(msgError, "Save failed: %v", err)
			return false
		}
		m.saved[s] = f.Format()
		m.dirty[s] = false
		written = append(written, m.shortName(side(s)))
	}
	m.recompute(m.selectedKey())
	if len(written) == 0 {
		m.setMessage(msgInfo, "No changes to save.")
	} else {
		m.setMessage(msgInfo, "Saved %s.", joinNames(written))
	}
	return true
}

func joinNames(names []string) string {
	if len(names) == 2 {
		return names[0] + " and " + names[1]
	}
	return names[0]
}

// recompute rebuilds the diff after any change. Entry pointers in the old
// diff may be stale, so this must run after every edit. The cursor stays on
// keepKey when it is still visible, otherwise at the same position.
func (m *Model) recompute(keepKey string) {
	m.set = diff.Compare(m.files[sideLeft], m.files[sideRight], m.opts)
	for s := range m.files {
		m.findings[s] = securityFindings(m.files[s])
	}
	m.rows = m.rows[:0]
	newCursor := -1
	for i, r := range m.set.Results {
		if m.onlyDiffs && r.Status == diff.StatusIdentical {
			continue
		}
		if r.Key == keepKey && keepKey != "" {
			newCursor = len(m.rows)
		}
		m.rows = append(m.rows, i)
	}
	if newCursor >= 0 {
		m.cursor = newCursor
	}
	m.clampScroll()
}

// securityFindings keeps the most severe WARNING-or-higher security finding
// for each key. audit.Audit sorts by severity, so the first one wins.
func securityFindings(f *envparse.EnvFile) map[string]audit.AuditFinding {
	out := map[string]audit.AuditFinding{}
	for _, fd := range audit.Audit(f, audit.DefaultOptions()) {
		if !strings.HasPrefix(fd.RuleID, "SEC") || fd.Severity.Rank() < audit.SeverityWarning.Rank() {
			continue
		}
		if _, seen := out[fd.Key]; !seen {
			out[fd.Key] = fd
		}
	}
	return out
}

// finding returns the most severe security finding for key across both
// files, and the side it was found in.
func (m *Model) finding(key string) (audit.AuditFinding, side, bool) {
	l, inL := m.findings[sideLeft][key]
	r, inR := m.findings[sideRight][key]
	switch {
	case inL && (!inR || l.Severity.Rank() >= r.Severity.Rank()):
		return l, sideLeft, true
	case inR:
		return r, sideRight, true
	}
	return audit.AuditFinding{}, sideLeft, false
}

func (m *Model) moveCursor(delta int) {
	m.cursor += delta
	m.clampScroll()
}

func (m *Model) clampScroll() {
	m.cursor = max(0, min(m.cursor, len(m.rows)-1))
	h := m.bodyHeight()
	if h <= 0 {
		m.offset = 0
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	m.offset = max(0, min(m.offset, len(m.rows)-h))
}
