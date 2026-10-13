package envparse

import (
	"reflect"
	"strings"
	"testing"
)

func kv(line int, key, value string) EnvEntry {
	return EnvEntry{Key: key, Value: value, RawValue: value, LineNumber: line, EndLine: line}
}

func quoted(line int, key, value, raw string, q QuoteType) EnvEntry {
	return EnvEntry{Key: key, Value: value, RawValue: raw, QuoteType: q, LineNumber: line, EndLine: line}
}

func blank(line int) EnvEntry {
	return EnvEntry{IsEmpty: true, LineNumber: line, EndLine: line}
}

func comment(line int, text string) EnvEntry {
	return EnvEntry{IsComment: true, RawComment: text, LineNumber: line, EndLine: line}
}

func with(e EnvEntry, mod func(*EnvEntry)) EnvEntry {
	mod(&e)
	return e
}

func TestParse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []EnvEntry
	}{
		{"empty input", "", []EnvEntry{}},
		{"simple", "A=1", []EnvEntry{kv(1, "A", "1")}},
		{"trailing newline adds no entry", "A=1\n", []EnvEntry{kv(1, "A", "1")}},
		{"multiple", "A=1\nB=2\n", []EnvEntry{kv(1, "A", "1"), kv(2, "B", "2")}},
		{"empty value", "KEY=", []EnvEntry{kv(1, "KEY", "")}},
		{"empty double quoted", `KEY=""`, []EnvEntry{quoted(1, "KEY", "", `""`, QuoteDouble)}},
		{"empty single quoted", `KEY=''`, []EnvEntry{quoted(1, "KEY", "", `''`, QuoteSingle)}},
		{"spaces around equals", "KEY  =  value", []EnvEntry{kv(1, "KEY", "value")}},
		{"tabs around equals", "KEY\t=\tvalue\t", []EnvEntry{kv(1, "KEY", "value")}},
		{"leading indentation", "   KEY=value", []EnvEntry{kv(1, "KEY", "value")}},
		{"trailing whitespace trimmed", "KEY=value   \t", []EnvEntry{kv(1, "KEY", "value")}},
		{"interior spaces kept", "KEY=hello big world", []EnvEntry{kv(1, "KEY", "hello big world")}},
		{"equals in value", "KEY=a=b=c", []EnvEntry{kv(1, "KEY", "a=b=c")}},
		{"dotted key", "app.port=80", []EnvEntry{kv(1, "app.port", "80")}},
		{"underscore key", "_PRIVATE_1=x", []EnvEntry{kv(1, "_PRIVATE_1", "x")}},
		{"unicode value", "NAME=héllo wörld ✓", []EnvEntry{kv(1, "NAME", "héllo wörld ✓")}},

		{"blank lines", "A=1\n\n  \nB=2", []EnvEntry{kv(1, "A", "1"), blank(2), blank(3), kv(4, "B", "2")}},
		{"only newlines", "\n\n", []EnvEntry{blank(1), blank(2)}},
		{"full line comment", "# hello", []EnvEntry{comment(1, "# hello")}},
		{"indented comment", "  # hello", []EnvEntry{comment(1, "# hello")}},
		{"comment then key", "#c\nA=1", []EnvEntry{comment(1, "#c"), kv(2, "A", "1")}},

		{"inline comment", "A=1 # note", []EnvEntry{with(kv(1, "A", "1"), func(e *EnvEntry) { e.RawComment = "# note" })}},
		{"inline comment after tab", "A=1\t#note", []EnvEntry{with(kv(1, "A", "1"), func(e *EnvEntry) { e.RawComment = "#note" })}},
		{"hash inside token", "A=a#b", []EnvEntry{kv(1, "A", "a#b")}},
		{"hash at value start", "COLOR=#ff0000", []EnvEntry{kv(1, "COLOR", "#ff0000")}},
		{"empty value with comment", "A= # none", []EnvEntry{with(kv(1, "A", ""), func(e *EnvEntry) { e.RawComment = "# none" })}},
		{"url fragment", "U=http://x/a#frag", []EnvEntry{kv(1, "U", "http://x/a#frag")}},

		{"export", "export A=1", []EnvEntry{with(kv(1, "A", "1"), func(e *EnvEntry) { e.IsExported = true })}},
		{"export extra spaces", "export \t A=1", []EnvEntry{with(kv(1, "A", "1"), func(e *EnvEntry) { e.IsExported = true })}},
		{"bare export", "export A", []EnvEntry{with(kv(1, "A", ""), func(e *EnvEntry) { e.IsExported, e.IsBareExport = true, true })}},
		{"bare export with comment", "export A # c", []EnvEntry{with(kv(1, "A", ""), func(e *EnvEntry) {
			e.IsExported, e.IsBareExport, e.RawComment = true, true, "# c"
		})}},
		{"key named export", "export=1", []EnvEntry{kv(1, "export", "1")}},
		{"key named export with spaces", "export = 1", []EnvEntry{kv(1, "export", "1")}},
		{"key prefixed export", "exporter=1", []EnvEntry{kv(1, "exporter", "1")}},

		{"single quoted literal", `A='$HOME \n'`, []EnvEntry{quoted(1, "A", `$HOME \n`, `'$HOME \n'`, QuoteSingle)}},
		{"backtick literal", "A=`it's \"fine\"`", []EnvEntry{quoted(1, "A", `it's "fine"`, "`it's \"fine\"`", QuoteBacktick)}},
		{"double quoted escapes", `A="a\nb\tc\rd\"e\\f\$g"`, []EnvEntry{quoted(1, "A", "a\nb\tc\rd\"e\\f$g", `"a\nb\tc\rd\"e\\f\$g"`, QuoteDouble)}},
		{"unknown escape kept", `A="C:\path"`, []EnvEntry{quoted(1, "A", `C:\path`, `"C:\path"`, QuoteDouble)}},
		{"double quoted keeps hash", `A="a # b"`, []EnvEntry{quoted(1, "A", "a # b", `"a # b"`, QuoteDouble)}},
		{"double quoted dollar unexpanded", `A="${B}"`, []EnvEntry{quoted(1, "A", "${B}", `"${B}"`, QuoteDouble)}},
		{"quoted with inline comment", `A="x" # c`, []EnvEntry{with(quoted(1, "A", "x", `"x"`, QuoteDouble), func(e *EnvEntry) { e.RawComment = "# c" })}},
		{"quoted with tight comment", `A='x'#c`, []EnvEntry{with(quoted(1, "A", "x", `'x'`, QuoteSingle), func(e *EnvEntry) { e.RawComment = "#c" })}},
		{"quoted with trailing space", "A='x'   ", []EnvEntry{quoted(1, "A", "x", `'x'`, QuoteSingle)}},
		{"other quotes inside double", `A="it's"`, []EnvEntry{quoted(1, "A", "it's", `"it's"`, QuoteDouble)}},
		{"unquoted with quote chars", `A=it's"ok"`, []EnvEntry{kv(1, "A", `it's"ok"`)}},
		{"backslash in unquoted", `A=C:\dir\n`, []EnvEntry{kv(1, "A", `C:\dir\n`)}},

		{"multiline double", "A=\"line1\nline2\"\nB=2", []EnvEntry{
			{Key: "A", Value: "line1\nline2", RawValue: "\"line1\nline2\"", QuoteType: QuoteDouble, LineNumber: 1, EndLine: 2},
			kv(3, "B", "2"),
		}},
		{"multiline single", "A='x\n\ny'", []EnvEntry{
			{Key: "A", Value: "x\n\ny", RawValue: "'x\n\ny'", QuoteType: QuoteSingle, LineNumber: 1, EndLine: 3},
		}},
		{"multiline backtick", "A=`x\ny`", []EnvEntry{
			{Key: "A", Value: "x\ny", RawValue: "`x\ny`", QuoteType: QuoteBacktick, LineNumber: 1, EndLine: 2},
		}},

		{"CRLF line endings", "A=1\r\nB=\"x\r\ny\"\r\n", []EnvEntry{
			kv(1, "A", "1"),
			{Key: "B", Value: "x\ny", RawValue: "\"x\ny\"", QuoteType: QuoteDouble, LineNumber: 2, EndLine: 3},
		}},
		{"UTF-8 BOM", "\xEF\xBB\xBFA=1", []EnvEntry{kv(1, "A", "1")}},
		{"duplicate keys kept", "A=1\nA=2", []EnvEntry{kv(1, "A", "1"), kv(2, "A", "2")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := ParseBytes([]byte(tt.input), "test.env")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(f.Entries, tt.want) {
				t.Errorf("entries mismatch\n got: %+v\nwant: %+v", f.Entries, tt.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		line, col int
		msg       string
	}{
		{"missing key", "=value", 1, 1, "missing key"},
		{"digit key start", "1A=x", 1, 1, "at start of key"},
		{"dash in key", "MY-KEY=x", 1, 3, "invalid character '-' in key"},
		{"space in key", "MY KEY=x", 1, 4, "unexpected 'K' after key"},
		{"no equals", "A=1\nJUSTKEY\n", 2, 8, "expected '='"},
		{"no equals at eof", "JUSTKEY", 1, 8, "expected '='"},
		{"export alone", "export", 1, 7, "expected '='"},
		{"unterminated double", "A=1\nB=\"abc\nC=3\n", 2, 3, "unterminated double-quoted"},
		{"unterminated single", "  K='abc", 1, 5, "unterminated single-quoted"},
		{"unterminated backtick", "K=`abc", 1, 3, "unterminated backtick-quoted"},
		{"trailing backslash in double", `K="abc\`, 1, 3, "unterminated double-quoted"},
		{"escaped closing quote", `K="abc\"`, 1, 3, "unterminated double-quoted"},
		{"junk after quote", `K="a"b`, 1, 6, "unexpected 'b' after closing quote"},
		{"junk after quote with space", `K='a' b`, 1, 7, "unexpected 'b' after closing quote"},
		{"column counts runes", "K='é' x", 1, 7, "after closing quote"},
		{"error on later line", "A=1\n\n# c\nB=\"x\" y", 4, 7, "after closing quote"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseBytes([]byte(tt.input), "bad.env")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			pe, ok := err.(*ParseError)
			if !ok {
				t.Fatalf("expected *ParseError, got %T: %v", err, err)
			}
			if pe.Line != tt.line || pe.Col != tt.col {
				t.Errorf("position = %d:%d, want %d:%d (%v)", pe.Line, pe.Col, tt.line, tt.col, err)
			}
			if !strings.Contains(pe.Msg, tt.msg) {
				t.Errorf("message %q does not contain %q", pe.Msg, tt.msg)
			}
			if pe.Path != "bad.env" || !strings.HasPrefix(err.Error(), "bad.env:") {
				t.Errorf("error %q missing path", err.Error())
			}
		})
	}
}

func TestIndexAndKeys(t *testing.T) {
	f, err := ParseBytes([]byte("# c\nB=1\nA=2\n\nB=3\nexport C\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := f.Keys(), []string{"B", "A", "C"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Keys() = %v, want %v", got, want)
	}
	e, ok := f.Get("B")
	if !ok || e.Value != "3" || e.LineNumber != 5 {
		t.Errorf("Get(B) = %+v, %v; want last occurrence with value 3", e, ok)
	}
	if e != &f.Entries[4] {
		t.Error("Index should point into Entries")
	}
	if _, ok := f.Get("missing"); ok {
		t.Error("Get(missing) should report false")
	}
	if len(f.Index) != 3 {
		t.Errorf("len(Index) = %d, want 3", len(f.Index))
	}
}

func TestReindexAfterAppend(t *testing.T) {
	f, err := ParseBytes([]byte("A=1"), "")
	if err != nil {
		t.Fatal(err)
	}
	f.Entries = append(f.Entries, kv(2, "B", "2"))
	f.Reindex()
	f.Entries[0].Value = "changed"
	if e, _ := f.Get("A"); e.Value != "changed" {
		t.Error("Index should point into the current Entries slice after Reindex")
	}
	if _, ok := f.Get("B"); !ok {
		t.Error("Reindex should pick up appended entries")
	}
}

func TestParseReaderAndFile(t *testing.T) {
	f, err := Parse(strings.NewReader("A=1\n"), "r.env")
	if err != nil {
		t.Fatal(err)
	}
	if f.Path != "r.env" || string(f.RawContent) != "A=1\n" {
		t.Errorf("Parse kept path %q and raw %q", f.Path, f.RawContent)
	}

	f, err = ParseFile("testdata/messy.env")
	if err != nil {
		t.Fatal(err)
	}
	if f.Path != "testdata/messy.env" || len(f.RawContent) == 0 {
		t.Errorf("ParseFile kept path %q and %d raw bytes", f.Path, len(f.RawContent))
	}

	if _, err := ParseFile("testdata/does-not-exist.env"); err == nil {
		t.Error("ParseFile on a missing file should fail")
	}
}

func TestQuoteTypeText(t *testing.T) {
	for _, q := range []QuoteType{QuoteNone, QuoteSingle, QuoteDouble, QuoteBacktick} {
		b, err := q.MarshalText()
		if err != nil {
			t.Fatal(err)
		}
		var got QuoteType
		if err := got.UnmarshalText(b); err != nil || got != q {
			t.Errorf("round trip %v -> %q -> %v (%v)", q, b, got, err)
		}
	}
	if _, err := QuoteType('x').MarshalText(); err == nil {
		t.Error("invalid QuoteType should not marshal")
	}
}
