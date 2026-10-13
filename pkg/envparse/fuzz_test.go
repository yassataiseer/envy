package envparse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func FuzzParse(f *testing.F) {
	seeds, _ := filepath.Glob("testdata/*.env")
	for _, s := range seeds {
		b, err := os.ReadFile(s)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	for _, s := range []string{
		"", "A=1", "export A", "A=\"x\\\"", "A='", "A=${", "A=${B:-${C:-}}",
		"\xEF\xBB\xBF#c\r\n", "A=\"\\", "=", "export =1", "A=`\n`#",
	} {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		file, err := ParseBytes(data, "fuzz.env")
		if err != nil {
			if _, ok := err.(*ParseError); !ok {
				t.Fatalf("non-ParseError returned: %T %v", err, err)
			}
			return
		}

		lines := strings.Count(normalize(data), "\n") + 1
		for i := range file.Entries {
			e := &file.Entries[i]
			if e.LineNumber < 1 || e.EndLine < e.LineNumber || e.EndLine > lines {
				t.Fatalf("entry %d has bad line span %d-%d (file has %d lines)", i, e.LineNumber, e.EndLine, lines)
			}
			kinds := 0
			for _, b := range []bool{e.IsAssignment(), e.IsComment, e.IsEmpty} {
				if b {
					kinds++
				}
			}
			if kinds != 1 {
				t.Fatalf("entry %d is not exactly one kind: %+v", i, e)
			}
			if e.QuoteType != QuoteNone {
				if len(e.RawValue) < 2 || rune(e.RawValue[0]) != rune(e.QuoteType) || rune(e.RawValue[len(e.RawValue)-1]) != rune(e.QuoteType) {
					t.Fatalf("entry %d raw value %q not wrapped in %v quotes", i, e.RawValue, e.QuoteType)
				}
			}
		}
		for _, k := range file.Keys() {
			if _, ok := file.Get(k); !ok {
				t.Fatalf("key %q missing from Index", k)
			}
		}

		if err := Expand(file, nil); err != nil {
			if _, ok := err.(*ExpandError); !ok {
				t.Fatalf("non-ExpandError returned: %T %v", err, err)
			}
		}
	})
}
