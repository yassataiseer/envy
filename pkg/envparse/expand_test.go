package envparse

import (
	"errors"
	"strings"
	"testing"
)

func mapLookup(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestExpand(t *testing.T) {
	env := map[string]string{"HOME": "/home/yassa", "EMPTY": "", "PATH": "/usr/bin"}
	tests := []struct {
		name  string
		input string
		key   string
		want  string
	}{
		{"bare reference", "A=$HOME/x", "A", "/home/yassa/x"},
		{"braced reference", "A=${HOME}x", "A", "/home/yassax"},
		{"bare name stops at non-ident", "A=$HOME.bak", "A", "/home/yassa.bak"},
		{"double quoted", `A="dir: ${HOME}"`, "A", "dir: /home/yassa"},
		{"single quoted untouched", `A='${HOME}'`, "A", "${HOME}"},
		{"backtick untouched", "A=`$HOME`", "A", "$HOME"},
		{"escaped dollar in double", `A="\${HOME}"`, "A", "${HOME}"},
		{"escaped dollar in unquoted", `A=\$HOME`, "A", "$HOME"},
		{"other backslashes unquoted", `A=C:\x\$HOME`, "A", `C:\x$HOME`},
		{"lone dollar", "A=costs $ 5", "A", "costs $ 5"},
		{"trailing dollar", "A=x$", "A", "x$"},
		{"dollar digit", "A=$1", "A", "$1"},
		{"unset expands empty", "A=[$NOPE]", "A", "[]"},
		{"earlier key", "B=hi\nA=${B} there", "A", "hi there"},
		{"chained", "X=1\nY=${X}2\nZ=\"${Y}3\"", "Z", "123"},
		{"file overrides env", "HOME=/override\nA=$HOME", "A", "/override"},
		{"self reference uses env", "PATH=$PATH:/opt/bin", "PATH", "/usr/bin:/opt/bin"},
		{"latest definition wins", "V=1\nA=$V\nV=2\nB=$V", "B", "2"},
		{"earlier read before redefinition", "V=1\nA=$V\nV=2\nB=$V", "A", "1"},
		{"forward reference not resolved", "A=${B}\nB=x", "A", ""},
		{"default when unset", "A=${NOPE:-fallback}", "A", "fallback"},
		{"default when empty", "A=${EMPTY:-fallback}", "A", "fallback"},
		{"no default when set", "A=${HOME:-fallback}", "A", "/home/yassa"},
		{"empty default", "A=${NOPE:-}", "A", ""},
		{"nested default", "A=${NOPE:-${ALSO_NOPE:-${HOME}}}", "A", "/home/yassa"},
		{"default with spaces", `A="${NOPE:-a b c}"`, "A", "a b c"},
		{"dotted braced key", "app.host=h\nA=${app.host}", "A", "h"},
		{"escapes still decoded", `A="${HOME}\n"`, "A", "/home/yassa\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := ParseBytes([]byte(tt.input), "")
			if err != nil {
				t.Fatal(err)
			}
			if err := Expand(f, mapLookup(env)); err != nil {
				t.Fatal(err)
			}
			var got string
			found := false
			for i := range f.Entries {
				if f.Entries[i].Key == tt.key {
					got, found = f.Entries[i].Value, true
					break
				}
			}
			if !found {
				t.Fatalf("key %s not found", tt.key)
			}
			if got != tt.want {
				t.Errorf("%s = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestExpandMutualReferencesTerminate(t *testing.T) {
	f, err := ParseBytes([]byte("A=${B}a\nB=${A}b"), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := Expand(f, nil); err != nil {
		t.Fatal(err)
	}
	a, _ := f.Get("A")
	b, _ := f.Get("B")
	if a.Value != "a" || b.Value != "ab" {
		t.Errorf("A=%q B=%q, want A=\"a\" B=\"ab\"", a.Value, b.Value)
	}
}

func TestExpandIdempotent(t *testing.T) {
	f, err := ParseBytes([]byte("A=x\nB=${A}${A}"), "")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := Expand(f, nil); err != nil {
			t.Fatal(err)
		}
	}
	if b, _ := f.Get("B"); b.Value != "xx" {
		t.Errorf("B = %q after two expansions, want \"xx\"", b.Value)
	}
}

func TestExpandSkipsBareExport(t *testing.T) {
	f, err := ParseBytes([]byte("export HOME\nA=$HOME"), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := Expand(f, mapLookup(map[string]string{"HOME": "/h"})); err != nil {
		t.Fatal(err)
	}
	if a, _ := f.Get("A"); a.Value != "/h" {
		t.Errorf("A = %q, want the environment's HOME", a.Value)
	}
	if h, _ := f.Get("HOME"); h.Value != "" {
		t.Errorf("bare export value = %q, want it left empty", h.Value)
	}
}

func TestExpandErrors(t *testing.T) {
	tests := []struct {
		name, input, msg string
		line             int
	}{
		{"unterminated brace", "A=${HOME", "unterminated ${HOME", 1},
		{"unterminated default", "X=1\nA=\"${HOME:-abc\"", "unterminated ${HOME:-", 2},
		{"empty name", "A=${}", "invalid variable name", 1},
		{"digit name", "A=${1}", "invalid variable name", 1},
		{"unsupported operator", "A=${HOME:=x}", "unsupported syntax", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := ParseBytes([]byte(tt.input), "e.env")
			if err != nil {
				t.Fatal(err)
			}
			before := make([]string, len(f.Entries))
			for i := range f.Entries {
				before[i] = f.Entries[i].Value
			}
			err = Expand(f, nil)
			var ee *ExpandError
			if !errors.As(err, &ee) {
				t.Fatalf("expected *ExpandError, got %T: %v", err, err)
			}
			if ee.Line != tt.line || ee.Key != "A" || !strings.Contains(ee.Msg, tt.msg) {
				t.Errorf("got %v (line %d, key %s), want line %d key A containing %q", err, ee.Line, ee.Key, tt.line, tt.msg)
			}
			for i := range f.Entries {
				if f.Entries[i].Value != before[i] {
					t.Errorf("entry %d modified despite error", i)
				}
			}
		})
	}
}
