package diff

import (
	"net/url"
	"strconv"
	"strings"
)

// ValueKind is the inferred type of a value, used to tell a changed value
// (8080 vs 3000) from a value of the wrong type (8080 vs "abc").
type ValueKind string

const (
	KindEmpty  ValueKind = "empty"
	KindBool   ValueKind = "bool"
	KindInt    ValueKind = "int"
	KindFloat  ValueKind = "float"
	KindURL    ValueKind = "url"
	KindString ValueKind = "string"
)

// InferKind classifies a value. "0" and "1" are ints, not bools.
func InferKind(v string) ValueKind {
	v = strings.TrimSpace(v)
	if v == "" {
		return KindEmpty
	}
	switch strings.ToLower(v) {
	case "true", "false", "yes", "no", "on", "off":
		return KindBool
	}
	if isNumeric(v) {
		if _, err := strconv.ParseInt(v, 10, 64); err == nil {
			return KindInt
		}
		if _, err := strconv.ParseFloat(v, 64); err == nil {
			return KindFloat
		}
	}
	if strings.Contains(v, "://") {
		if u, err := url.Parse(v); err == nil && u.Scheme != "" && (u.Host != "" || u.Opaque != "" || u.Path != "") {
			return KindURL
		}
	}
	return KindString
}

// isNumeric rejects inputs like "inf", "NaN", or "0x1F" that strconv would
// otherwise accept.
func isNumeric(v string) bool {
	digits := 0
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case c >= '0' && c <= '9':
			digits++
		case c == '.' || c == 'e' || c == 'E' || ((c == '-' || c == '+') && (i == 0 || v[i-1] == 'e' || v[i-1] == 'E')):
		default:
			return false
		}
	}
	return digits > 0
}

// kindsConflict reports whether two non-empty values have incompatible
// types. Ints and floats are compatible.
func kindsConflict(a, b ValueKind) bool {
	if a == KindEmpty || b == KindEmpty || a == b {
		return false
	}
	numeric := func(k ValueKind) bool { return k == KindInt || k == KindFloat }
	return !(numeric(a) && numeric(b))
}
