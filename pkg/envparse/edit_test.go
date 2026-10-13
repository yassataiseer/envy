package envparse

import (
	"os"
	"path/filepath"
	"testing"
)

func mustParse(t *testing.T, s string) *EnvFile {
	t.Helper()
	f, err := ParseBytes([]byte(s), "t.env")
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestFormatRoundTripsTestdata(t *testing.T) {
	files, _ := filepath.Glob("testdata/*.env")
	for _, path := range files {
		f, err := ParseFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := string(f.Format()); got != string(f.RawContent) {
			t.Errorf("%s did not round-trip:\n got: %q\nwant: %q", path, got, f.RawContent)
		}
	}
}

func TestFormatRoundTripsEdgeCases(t *testing.T) {
	for _, in := range []string{
		"", "A=1", "A=1\n", "\n\n", "  KEY = v   # c  \n", "A=1\n   ",
		"A=\"multi\nline\"  # c\nB=2", "\xEF\xBB\xBFA=1\r\nB=2\r\n", "export A\n",
	} {
		f := mustParse(t, in)
		if got := string(f.Format()); got != in {
			t.Errorf("round trip of %q gave %q", in, got)
		}
	}
}

func TestSetUpdatesInPlace(t *testing.T) {
	f := mustParse(t, "# header\nA = 1   # keep me\nB='x'\n\nC=3\n")
	if err := f.Set("A", "two words"); err != nil {
		t.Fatal(err)
	}
	want := "# header\nA=\"two words\" # keep me\nB='x'\n\nC=3\n"
	if got := string(f.Format()); got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
	if e, _ := f.Get("A"); !e.IsModified() || e.Value != "two words" {
		t.Errorf("A = %+v", e)
	}
}

func TestSetUpdatesLastDuplicate(t *testing.T) {
	f := mustParse(t, "A=1\nA=2\n")
	if err := f.Set("A", "3"); err != nil {
		t.Fatal(err)
	}
	if got := string(f.Format()); got != "A=1\nA=3\n" {
		t.Errorf("got %q", got)
	}
}

func TestSetAppends(t *testing.T) {
	tests := []struct{ in, want string }{
		{"A=1\n", "A=1\nB=hello\n"},
		{"A=1", "A=1\nB=hello\n"},
		{"", "B=hello\n"},
		{"A=1\r\n", "A=1\r\nB=hello\r\n"},
	}
	for _, tt := range tests {
		f := mustParse(t, tt.in)
		if err := f.Set("B", "hello"); err != nil {
			t.Fatal(err)
		}
		if got := string(f.Format()); got != tt.want {
			t.Errorf("append to %q: got %q, want %q", tt.in, got, tt.want)
		}
		if _, ok := f.Get("B"); !ok {
			t.Errorf("append to %q: B not indexed", tt.in)
		}
	}
}

func TestSetRejectsInvalidKey(t *testing.T) {
	f := mustParse(t, "")
	for _, k := range []string{"", "1A", "A-B", "A B"} {
		if err := f.Set(k, "v"); err == nil {
			t.Errorf("Set(%q) should fail", k)
		}
	}
}

func TestSetFromPreservesRaw(t *testing.T) {
	src := mustParse(t, "export URL=\"http://${HOST}\"\nKEY='lit $x'\n")
	dst := mustParse(t, "URL=old\n")
	u, _ := src.Get("URL")
	k, _ := src.Get("KEY")
	if err := dst.SetFrom(u); err != nil {
		t.Fatal(err)
	}
	if err := dst.SetFrom(k); err != nil {
		t.Fatal(err)
	}
	want := "URL=\"http://${HOST}\"\nKEY='lit $x'\n"
	if got := string(dst.Format()); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if err := dst.SetFrom(&EnvEntry{IsComment: true, RawComment: "# x"}); err == nil {
		t.Error("SetFrom with a comment entry should fail")
	}
}

func TestSetFromBareExport(t *testing.T) {
	src := mustParse(t, "export PATH\n")
	dst := mustParse(t, "")
	p, _ := src.Get("PATH")
	if err := dst.SetFrom(p); err != nil {
		t.Fatal(err)
	}
	if got := string(dst.Format()); got != "export PATH\n" {
		t.Errorf("got %q", got)
	}
}

func TestDelete(t *testing.T) {
	f := mustParse(t, "A=1\nB=2 # c\nA=3\nC=4")
	if !f.Delete("A") {
		t.Fatal("Delete(A) reported nothing removed")
	}
	if f.Delete("missing") {
		t.Error("Delete(missing) reported a removal")
	}
	if got := string(f.Format()); got != "B=2 # c\nC=4" {
		t.Errorf("got %q", got)
	}
	if _, ok := f.Get("A"); ok {
		t.Error("A still indexed")
	}
	if c, _ := f.Get("C"); c.Value != "4" {
		t.Error("index not rebuilt after delete")
	}
}

func TestQuoteValue(t *testing.T) {
	tests := []struct {
		in, raw string
		q       QuoteType
	}{
		{"", "", QuoteNone},
		{"simple", "simple", QuoteNone},
		{"postgres://u:p@h:5432/db?x=1", "postgres://u:p@h:5432/db?x=1", QuoteNone},
		{"has space", `"has space"`, QuoteDouble},
		{"#ff0000", `"#ff0000"`, QuoteDouble},
		{"$HOME", `"\$HOME"`, QuoteDouble},
		{`say "hi"`, `"say \"hi\""`, QuoteDouble},
		{`C:\dir`, `"C:\\dir"`, QuoteDouble},
		{"line1\nline2", `"line1\nline2"`, QuoteDouble},
		{"it's", `"it's"`, QuoteDouble},
		{"héllo", "héllo", QuoteNone},
	}
	for _, tt := range tests {
		raw, q := QuoteValue(tt.in)
		if raw != tt.raw || q != tt.q {
			t.Errorf("QuoteValue(%q) = %s %v, want %s %v", tt.in, raw, q, tt.raw, tt.q)
		}
	}
}

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("# keep\nA=1\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	f, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Set("B", "new value"); err != nil {
		t.Fatal(err)
	}
	if err := f.WriteFile(path); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(path)
	if want := "# keep\nA=1\nB=\"new value\"\n"; string(got) != want {
		t.Errorf("file = %q, want %q", got, want)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o640 {
		t.Errorf("perm = %v, want 0640", info.Mode().Perm())
	}
	b, _ := f.Get("B")
	if b.IsModified() || b.LineNumber != 3 || string(f.RawContent) != string(got) {
		t.Errorf("file not re-parsed after write: %+v", b)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("temp files left behind: %v", entries)
	}

	newPath := filepath.Join(dir, "new.env")
	nf := mustParse(t, "")
	_ = nf.Set("X", "1")
	if err := nf.WriteFile(newPath); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(newPath); info.Mode().Perm() != 0o600 {
		t.Errorf("new file perm = %v, want 0600", info.Mode().Perm())
	}
}
