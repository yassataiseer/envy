package envparse

import (
	"reflect"
	"testing"
)

func TestLexTokenStream(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []tokenKind
	}{
		{"empty", "", []tokenKind{tokEOF}},
		{"assignment", "A=1", []tokenKind{tokKey, tokAssign, tokValue, tokEOF}},
		{"export with comment", "export A='x' # c\n", []tokenKind{tokExport, tokKey, tokAssign, tokValue, tokComment, tokNewline, tokEOF}},
		{"bare export", "export A\n", []tokenKind{tokExport, tokKey, tokNewline, tokEOF}},
		{"blank and comment", "\n# c", []tokenKind{tokNewline, tokComment, tokEOF}},
		{"empty value", "A=\n", []tokenKind{tokKey, tokAssign, tokValue, tokNewline, tokEOF}},
		{"multiline value is one token", "A=\"1\n2\"\nB=3", []tokenKind{tokKey, tokAssign, tokValue, tokNewline, tokKey, tokAssign, tokValue, tokEOF}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toks, err := lex("", tt.input)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]tokenKind, len(toks))
			for i, tok := range toks {
				got[i] = tok.kind
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("kinds = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLexTokenPositions(t *testing.T) {
	toks, err := lex("", "  export KEY = \"a\nb\" #c\nX=1")
	if err != nil {
		t.Fatal(err)
	}
	want := []token{
		{kind: tokExport, text: "export", line: 1, col: 3, endLine: 1},
		{kind: tokKey, text: "KEY", line: 1, col: 10, endLine: 1},
		{kind: tokAssign, text: "=", line: 1, col: 14, endLine: 1},
		{kind: tokValue, text: "\"a\nb\"", value: "a\nb", quote: QuoteDouble, line: 1, col: 16, endLine: 2},
		{kind: tokComment, text: "#c", line: 2, col: 4, endLine: 2},
		{kind: tokNewline, text: "\n", line: 2, col: 6, endLine: 3},
		{kind: tokKey, text: "X", line: 3, col: 1, endLine: 3},
		{kind: tokAssign, text: "=", line: 3, col: 2, endLine: 3},
		{kind: tokValue, text: "1", value: "1", line: 3, col: 3, endLine: 3},
		{kind: tokEOF, text: "", line: 3, col: 4, endLine: 3},
	}
	if !reflect.DeepEqual(toks, want) {
		for i := range max(len(toks), len(want)) {
			var g, w token
			if i < len(toks) {
				g = toks[i]
			}
			if i < len(want) {
				w = want[i]
			}
			if g != w {
				t.Errorf("token %d:\n got %+v\nwant %+v", i, g, w)
			}
		}
	}
}
