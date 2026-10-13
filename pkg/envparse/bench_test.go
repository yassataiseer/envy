package envparse

import (
	"fmt"
	"strings"
	"testing"
)

// benchInput builds a representative 500-line file mixing every syntax form.
func benchInput() []byte {
	var b strings.Builder
	for i := 0; strings.Count(b.String(), "\n") < 500; i++ {
		switch i % 10 {
		case 0:
			fmt.Fprintf(&b, "# section %d\n", i)
		case 1:
			b.WriteString("\n")
		case 2:
			fmt.Fprintf(&b, "export KEY_%d=plain_value_%d\n", i, i)
		case 3:
			fmt.Fprintf(&b, "KEY_%d=\"double \\t quoted ${KEY_%d} \\n value\"\n", i, i-1)
		case 4:
			fmt.Fprintf(&b, "KEY_%d='single quoted $literal'\n", i)
		case 5:
			fmt.Fprintf(&b, "KEY_%d = spaced value # inline comment\n", i)
		case 6:
			fmt.Fprintf(&b, "KEY_%d=\"multi\nline\nvalue\"\n", i)
		case 7:
			fmt.Fprintf(&b, "KEY_%d=sk_test_4eC39HqL_yjWDarjt_T1zdp7dc%d\n", i, i)
		case 8:
			fmt.Fprintf(&b, "KEY_%d=${KEY_%d:-fallback}\n", i, i-6)
		case 9:
			fmt.Fprintf(&b, "KEY_%d=`backtick value`\n", i)
		}
	}
	return []byte(b.String())
}

func BenchmarkParse(b *testing.B) {
	input := benchInput()
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ParseBytes(input, "bench.env"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseAndExpand(b *testing.B) {
	input := benchInput()
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	for b.Loop() {
		f, err := ParseBytes(input, "bench.env")
		if err != nil {
			b.Fatal(err)
		}
		if err := Expand(f, nil); err != nil {
			b.Fatal(err)
		}
	}
}
