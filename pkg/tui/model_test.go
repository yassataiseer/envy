package tui

import (
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/yassataiseer/envy/pkg/diff"
	"github.com/yassataiseer/envy/pkg/envparse"
)

const (
	testLeft = `PORT=8080
DATABASE_URL=postgres://localhost/app
NEXT_PUBLIC_STRIPE_SECRET=sk_123
`
	testRight = `PORT=8080
DATABASE_URL=
LOG_LEVEL=debug
`
)

type fixture struct {
	m                   Model
	leftPath, rightPath string
}

func newFixture(t *testing.T, left, right string) *fixture {
	t.Helper()
	dir := t.TempDir()
	fx := &fixture{leftPath: filepath.Join(dir, ".env"), rightPath: filepath.Join(dir, ".env.example")}
	for path, content := range map[string]string{fx.leftPath: left, fx.rightPath: right} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	l, err := envparse.ParseFile(fx.leftPath)
	if err != nil {
		t.Fatal(err)
	}
	r, err := envparse.ParseFile(fx.rightPath)
	if err != nil {
		t.Fatal(err)
	}
	fx.m = New(Config{Left: l, Right: r})
	fx.send(tea.WindowSizeMsg{Width: 100, Height: 16})
	return fx
}

func keyMsg(s string) tea.KeyPressMsg {
	switch s {
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	r := []rune(s)
	if len(r) != 1 {
		panic("unsupported test key " + s)
	}
	return tea.KeyPressMsg{Code: r[0], Text: s}
}

func (fx *fixture) send(msg tea.Msg) tea.Cmd {
	next, cmd := fx.m.Update(msg)
	fx.m = next.(Model)
	return cmd
}

func (fx *fixture) press(keys ...string) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range keys {
		cmd = fx.send(keyMsg(k))
	}
	return cmd
}

func (fx *fixture) selectKey(t *testing.T, key string) {
	t.Helper()
	fx.press("g")
	for range fx.m.rows {
		if fx.m.selectedKey() == key {
			return
		}
		fx.press("j")
	}
	t.Fatalf("key %s not found in rows", key)
}

func (fx *fixture) screen() string {
	return ansi.Strip(fx.m.View().Content)
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInitialView(t *testing.T) {
	fx := newFixture(t, testLeft, testRight)
	s := fx.screen()
	for _, want := range []string{
		".env", ".env.example",
		"PORT=••••••••",
		"MISSING KEY",
		"LOG_LEVEL=••••••••",
		"4 keys",
		"~ 1 changed",
		"- 1 missing in",
		"+ 1 missing in",
		"[masked]",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("screen missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "sk_123") || strings.Contains(s, "8080") {
		t.Errorf("secret values visible while masked:\n%s", s)
	}
	v := fx.m.View()
	if !v.AltScreen {
		t.Error("view should use the alternate screen")
	}
}

func TestLayoutFitsWindow(t *testing.T) {
	fx := newFixture(t, testLeft, testRight)
	for _, size := range [][2]int{{100, 16}, {40, 8}, {57, 30}, {200, 10}} {
		fx.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		fx.press("?")
		for _, showAll := range []bool{true, false} {
			lines := strings.Split(fx.screen(), "\n")
			if len(lines) > size[1] {
				t.Errorf("%dx%d (full help %v): %d lines", size[0], size[1], showAll, len(lines))
			}
			for i, l := range lines {
				if w := ansi.StringWidth(l); w > size[0] {
					t.Errorf("%dx%d: line %d is %d wide: %q", size[0], size[1], i, w, l)
				}
			}
			fx.press("?")
		}
	}
}

func TestTooSmall(t *testing.T) {
	fx := newFixture(t, testLeft, testRight)
	fx.send(tea.WindowSizeMsg{Width: 30, Height: 5})
	if !strings.Contains(fx.screen(), "Terminal too small") {
		t.Errorf("screen = %q", fx.screen())
	}
}

func TestRevealToggle(t *testing.T) {
	fx := newFixture(t, testLeft, testRight)
	fx.press("m")
	s := fx.screen()
	if !strings.Contains(s, "PORT=8080") || !strings.Contains(s, "[REVEALED]") {
		t.Errorf("values not revealed:\n%s", s)
	}
	fx.press("m")
	if strings.Contains(fx.screen(), "8080") {
		t.Error("values not masked again")
	}
}

func TestSanitizeControlCharacters(t *testing.T) {
	fx := newFixture(t, "EVIL=\"a\\nb\"\nESC=x\x1b[6n\x1b]8;;http://evil\x07y\n", "")
	fx.press("m")
	s := fx.m.View().Content
	if strings.Contains(s, "\x1b[6n") || strings.Contains(s, "\x1b]8;;http://evil") {
		t.Error("raw escape sequence from a value reached the screen")
	}
	if !strings.Contains(ansi.Strip(s), "EVIL=a⏎b") {
		t.Errorf("newline not rendered visibly:\n%s", ansi.Strip(s))
	}
}

func TestNavigation(t *testing.T) {
	fx := newFixture(t, testLeft, testRight)
	keyAt := func() string { return fx.m.selectedKey() }
	if keyAt() != "PORT" {
		t.Fatalf("initial selection = %s", keyAt())
	}
	// LOG_LEVEL exists only on the right, after DATABASE_URL, so it is
	// listed next to it.
	fx.press("j", "down")
	if keyAt() != "LOG_LEVEL" {
		t.Errorf("after 2 down = %s", keyAt())
	}
	fx.press("G")
	if keyAt() != "NEXT_PUBLIC_STRIPE_SECRET" {
		t.Errorf("after G = %s", keyAt())
	}
	fx.press("j")
	if keyAt() != "NEXT_PUBLIC_STRIPE_SECRET" {
		t.Errorf("cursor moved past the end: %s", keyAt())
	}
	fx.press("g", "k")
	if keyAt() != "PORT" {
		t.Errorf("cursor moved before the start: %s", keyAt())
	}
}

func TestScrollKeepsCursorVisible(t *testing.T) {
	var b strings.Builder
	for i := range 50 {
		b.WriteString("K" + string(rune('A'+i/26)) + string(rune('A'+i%26)) + "=1\n")
	}
	fx := newFixture(t, b.String(), "")
	for range 30 {
		fx.press("j")
	}
	if !strings.Contains(fx.screen(), fx.m.selectedKey()+"=") {
		t.Errorf("selected key %s not on screen:\n%s", fx.m.selectedKey(), fx.screen())
	}
	if fx.m.offset == 0 {
		t.Error("view did not scroll")
	}
}

func TestFilterOnlyDiffs(t *testing.T) {
	fx := newFixture(t, testLeft, testRight)
	fx.press("f")
	if len(fx.m.rows) != 3 {
		t.Errorf("rows = %d, want 3 differing keys", len(fx.m.rows))
	}
	if fx.m.selectedKey() == "PORT" {
		t.Error("identical key still selected after filtering")
	}
	if !strings.Contains(fx.screen(), "[diffs only]") {
		t.Error("filter flag not shown")
	}
	fx.press("f")
	if len(fx.m.rows) != 4 {
		t.Errorf("rows = %d after unfiltering", len(fx.m.rows))
	}
}

func TestFilterEmptyState(t *testing.T) {
	fx := newFixture(t, "A=1\n", "A=1\n")
	fx.press("f")
	if !strings.Contains(fx.screen(), "No differences") {
		t.Errorf("screen:\n%s", fx.screen())
	}
	fx.press("space", "d", "j")
}

func TestCopyMissingKeyToRightAndSave(t *testing.T) {
	fx := newFixture(t, testLeft, testRight)
	fx.selectKey(t, "NEXT_PUBLIC_STRIPE_SECRET") // missing on the right
	fx.press("space")
	r, _ := fx.m.selected()
	if r.Key != "NEXT_PUBLIC_STRIPE_SECRET" || r.Status != diff.StatusIdentical {
		t.Fatalf("after copy: %+v", r)
	}
	if !fx.m.dirty[sideRight] || fx.m.dirty[sideLeft] {
		t.Errorf("dirty = %v", fx.m.dirty)
	}
	if !strings.Contains(fx.screen(), ".env.example (modified)") {
		t.Error("modified marker missing")
	}
	if readFile(t, fx.rightPath) != testRight {
		t.Fatal("file written before save")
	}

	fx.press("s")
	if got, want := readFile(t, fx.rightPath), testRight+"NEXT_PUBLIC_STRIPE_SECRET=sk_123\n"; got != want {
		t.Errorf("right file = %q, want %q", got, want)
	}
	if readFile(t, fx.leftPath) != testLeft {
		t.Error("left file changed")
	}
	if fx.m.dirty[sideRight] || !strings.Contains(fx.screen(), "Saved .env.example") {
		t.Errorf("not marked saved:\n%s", fx.screen())
	}
}

func TestCopyDirections(t *testing.T) {
	fx := newFixture(t, testLeft, testRight)
	fx.press("j") // DATABASE_URL: value differs
	fx.press("<")
	if l, _ := fx.m.files[sideLeft].Get("DATABASE_URL"); l.Value != "" {
		t.Errorf("< should copy right to left, left = %q", l.Value)
	}
	fx.press("u")
	if l, _ := fx.m.files[sideLeft].Get("DATABASE_URL"); l.Value != "postgres://localhost/app" {
		t.Errorf("undo did not restore left, got %q", l.Value)
	}
	if fx.m.dirty[sideLeft] {
		t.Error("left still dirty after undo")
	}
	fx.press(">")
	if r, _ := fx.m.files[sideRight].Get("DATABASE_URL"); r.Value != "postgres://localhost/app" {
		t.Errorf("> should copy left to right, right = %q", r.Value)
	}
}

func TestCopyFromMissingSideWarns(t *testing.T) {
	fx := newFixture(t, testLeft, testRight)
	fx.selectKey(t, "LOG_LEVEL") // missing on the left
	fx.press("space")
	if !strings.Contains(fx.screen(), "nothing to copy") {
		t.Errorf("screen:\n%s", fx.screen())
	}
	fx.press("tab", "space")
	if _, ok := fx.m.files[sideLeft].Get("LOG_LEVEL"); !ok {
		t.Error("copy from focused right pane failed")
	}
}

func TestDeleteFromFocusedPane(t *testing.T) {
	fx := newFixture(t, testLeft, testRight)
	fx.selectKey(t, "LOG_LEVEL")
	fx.press("l", "d")
	if _, ok := fx.m.files[sideRight].Get("LOG_LEVEL"); ok {
		t.Error("LOG_LEVEL not deleted from right")
	}
	for _, r := range fx.m.set.Results {
		if r.Key == "LOG_LEVEL" {
			t.Error("deleted key still listed")
		}
	}
	fx.press("u")
	if _, ok := fx.m.files[sideRight].Get("LOG_LEVEL"); !ok {
		t.Error("undo did not restore deleted key")
	}
}

func TestQuitFlow(t *testing.T) {
	fx := newFixture(t, testLeft, testRight)
	if !isQuit(fx.press("q")) {
		t.Fatal("q should quit when nothing changed")
	}

	fx = newFixture(t, testLeft, testRight)
	fx.selectKey(t, "NEXT_PUBLIC_STRIPE_SECRET")
	fx.press("space")
	if isQuit(fx.press("q")) {
		t.Fatal("q should ask for confirmation with unsaved changes")
	}
	if !strings.Contains(fx.screen(), "Unsaved changes") {
		t.Errorf("no confirmation prompt:\n%s", fx.screen())
	}
	if isQuit(fx.press("j")) || fx.m.confirmQuit {
		t.Fatal("another key should cancel quitting")
	}
	fx.press("q")
	if !isQuit(fx.press("q")) {
		t.Fatal("q twice should discard and quit")
	}
	if readFile(t, fx.rightPath) != testRight {
		t.Error("discarded changes were written")
	}

	fx = newFixture(t, testLeft, testRight)
	fx.selectKey(t, "NEXT_PUBLIC_STRIPE_SECRET")
	fx.press("space", "q")
	if !isQuit(fx.press("s")) {
		t.Fatal("s at the prompt should save and quit")
	}
	if !strings.Contains(readFile(t, fx.rightPath), "NEXT_PUBLIC_STRIPE_SECRET") {
		t.Error("save-and-quit did not write")
	}

	fx = newFixture(t, testLeft, testRight)
	fx.selectKey(t, "NEXT_PUBLIC_STRIPE_SECRET")
	fx.press("space")
	if !isQuit(fx.press("ctrl+c")) {
		t.Fatal("ctrl+c should always quit")
	}
}

func TestBackgroundColorMsg(t *testing.T) {
	fx := newFixture(t, testLeft, testRight)
	if fx.m.Init() == nil {
		t.Error("Init should request the background color")
	}
	fx.send(tea.BackgroundColorMsg{Color: color.White})
	if fx.m.isDark {
		t.Error("white background should switch to the light theme")
	}
	fx.send(tea.BackgroundColorMsg{Color: color.Black})
	if !fx.m.isDark {
		t.Error("black background should switch to the dark theme")
	}
}

func TestHighContrastRenders(t *testing.T) {
	l, _ := envparse.ParseBytes([]byte(testLeft), "l")
	r, _ := envparse.ParseBytes([]byte(testRight), "r")
	m := New(Config{Left: l, Right: r, HighContrast: true, Reveal: true})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	s := ansi.Strip(next.(Model).View().Content)
	if !strings.Contains(s, "PORT=8080") {
		t.Errorf("screen:\n%s", s)
	}
}
