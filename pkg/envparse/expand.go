package envparse

import (
	"errors"
	"strings"
)

// Expand resolves $VAR, ${VAR}, and ${VAR:-default} references in unquoted
// and double-quoted values, updating each entry's Value. Single-quoted and
// backtick values are never expanded. A backslash before '$' (`\$`) yields
// a literal dollar sign.
//
// As in a shell, a reference resolves to the most recent definition of the
// key earlier in the file, falling back to lookup (which may be nil, e.g.
// os.LookupEnv). Forward references are not resolved against the file, so
// `PATH=$PATH:/extra` refers to the outer PATH and reference cycles cannot
// occur. Unset variables expand to the empty string. ${VAR:-default} uses
// default when VAR is unset or empty.
//
// Values are re-derived from RawValue, so Expand is idempotent. On error,
// f is left unchanged.
func Expand(f *EnvFile, lookup func(string) (string, bool)) error {
	defined := make(map[string]string, len(f.Index))
	resolve := func(name string) (string, bool) {
		if v, ok := defined[name]; ok {
			return v, true
		}
		if lookup != nil {
			return lookup(name)
		}
		return "", false
	}

	values := make([]string, len(f.Entries))
	for i := range f.Entries {
		e := &f.Entries[i]
		if !e.IsAssignment() || e.IsBareExport {
			continue
		}
		v, err := render(innerValue(e), e.QuoteType, resolve)
		if err != nil {
			return &ExpandError{Path: f.Path, Key: e.Key, Line: e.LineNumber, Msg: err.Error()}
		}
		values[i] = v
		defined[e.Key] = v
	}

	for i := range f.Entries {
		e := &f.Entries[i]
		if e.IsAssignment() && !e.IsBareExport {
			e.Value = values[i]
		}
	}
	return nil
}

// innerValue returns RawValue without its surrounding quotes.
func innerValue(e *EnvEntry) string {
	if e.QuoteType == QuoteNone || len(e.RawValue) < 2 {
		return e.RawValue
	}
	return e.RawValue[1 : len(e.RawValue)-1]
}

// render decodes the inner text of a value according to its quoting.
// Double-quoted text has its escape sequences decoded. When resolve is
// non-nil, variable references in unquoted and double-quoted text are
// expanded as well; when it is nil, render never fails.
func render(s string, q QuoteType, resolve func(string) (string, bool)) (string, error) {
	if q == QuoteSingle || q == QuoteBacktick || (q == QuoteNone && resolve == nil) {
		return s, nil
	}
	if strings.IndexByte(s, '\\') < 0 && (resolve == nil || strings.IndexByte(s, '$') < 0) {
		return s, nil
	}

	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s):
			if decoded, ok := decodeEscape(s[i+1], q); ok {
				b.WriteByte(decoded)
				i++
			} else {
				b.WriteByte('\\')
			}
		case c == '$' && resolve != nil:
			val, n, err := expandRef(s[i:], q, resolve)
			if err != nil {
				return "", err
			}
			b.WriteString(val)
			i += n - 1
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), nil
}

// decodeEscape returns the byte that the escape sequence `\c` stands for.
// Unrecognized escapes are kept verbatim by the caller.
func decodeEscape(c byte, q QuoteType) (byte, bool) {
	if q == QuoteNone {
		return '$', c == '$'
	}
	switch c {
	case 'n':
		return '\n', true
	case 't':
		return '\t', true
	case 'r':
		return '\r', true
	case '"', '\\', '$':
		return c, true
	}
	return 0, false
}

// expandRef expands the variable reference at the start of s, which begins
// with '$'. It returns the expansion and the number of bytes consumed. A '$'
// that does not start a reference is returned literally.
func expandRef(s string, q QuoteType, resolve func(string) (string, bool)) (string, int, error) {
	if len(s) < 2 {
		return "$", 1, nil
	}
	if isKeyStart(rune(s[1])) {
		j := 2
		for j < len(s) && isIdentChar(s[j]) {
			j++
		}
		v, _ := resolve(s[1:j])
		return v, j, nil
	}
	if s[1] != '{' {
		return "$", 1, nil
	}

	j := 2
	for j < len(s) && isKeyChar(rune(s[j])) {
		j++
	}
	name := s[2:j]
	if name == "" || !isKeyStart(rune(name[0])) {
		return "", 0, errors.New("invalid variable name in ${...}")
	}
	switch {
	case j < len(s) && s[j] == '}':
		v, _ := resolve(name)
		return v, j + 1, nil
	case strings.HasPrefix(s[j:], ":-"):
		end := matchingBrace(s, j+2)
		if end < 0 {
			return "", 0, errors.New("unterminated ${" + name + ":-...}")
		}
		if v, ok := resolve(name); ok && v != "" {
			return v, end + 1, nil
		}
		def, err := render(s[j+2:end], q, resolve)
		if err != nil {
			return "", 0, err
		}
		return def, end + 1, nil
	case j >= len(s):
		return "", 0, errors.New("unterminated ${" + name)
	default:
		return "", 0, errors.New("unsupported syntax in ${" + name + "...}; only ${VAR} and ${VAR:-default} are supported")
	}
}

// matchingBrace returns the index of the '}' that closes a ${ opened before
// start, accounting for nested ${...} and backslash escapes, or -1.
func matchingBrace(s string, start int) int {
	depth := 1
	for k := start; k < len(s); k++ {
		switch {
		case s[k] == '\\':
			k++
		case s[k] == '$' && k+1 < len(s) && s[k+1] == '{':
			depth++
			k++
		case s[k] == '}':
			depth--
			if depth == 0 {
				return k
			}
		}
	}
	return -1
}

func isIdentChar(c byte) bool {
	return c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
}
