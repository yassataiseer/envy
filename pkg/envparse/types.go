// Package envparse implements a regex-free, finite-state parser for .env files.
//
// Parsing preserves the structure of the file (comments, blank lines, quoting,
// export prefixes, and line numbers) so that callers can diff, audit, or
// rewrite files without losing information.
package envparse

import "fmt"

// QuoteType defines the encapsulation mode of a variable value.
type QuoteType rune

const (
	QuoteNone     QuoteType = 0
	QuoteSingle   QuoteType = '\''
	QuoteDouble   QuoteType = '"'
	QuoteBacktick QuoteType = '`'
)

func (q QuoteType) String() string {
	switch q {
	case QuoteNone:
		return "none"
	case QuoteSingle:
		return "single"
	case QuoteDouble:
		return "double"
	case QuoteBacktick:
		return "backtick"
	default:
		return fmt.Sprintf("QuoteType(%d)", rune(q))
	}
}

func (q QuoteType) MarshalText() ([]byte, error) {
	switch q {
	case QuoteNone, QuoteSingle, QuoteDouble, QuoteBacktick:
		return []byte(q.String()), nil
	default:
		return nil, fmt.Errorf("envparse: invalid QuoteType %d", rune(q))
	}
}

func (q *QuoteType) UnmarshalText(b []byte) error {
	switch string(b) {
	case "none", "":
		*q = QuoteNone
	case "single":
		*q = QuoteSingle
	case "double":
		*q = QuoteDouble
	case "backtick":
		*q = QuoteBacktick
	default:
		return fmt.Errorf("envparse: unknown quote type %q", b)
	}
	return nil
}

// EnvEntry represents a single logical line or record in a .env file.
//
// Exactly one of the following holds: the entry is a key/value assignment
// (Key != ""), a full-line comment (IsComment), or a blank line (IsEmpty).
type EnvEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	// RawValue is the value exactly as written in the source, including
	// surrounding quotes. Unquoted values have surrounding whitespace removed.
	RawValue string `json:"raw_value"`
	// RawComment holds the comment text starting at '#'. For comment entries
	// it is the whole comment; for assignments it is the inline comment.
	RawComment string    `json:"raw_comment,omitempty"`
	LineNumber int       `json:"line_number"`
	EndLine    int       `json:"end_line"`
	QuoteType  QuoteType `json:"quote_type"`
	IsExported bool      `json:"is_exported"`
	// IsBareExport marks `export KEY` with no assignment, which re-exports
	// an existing variable rather than setting it to the empty string.
	IsBareExport bool `json:"is_bare_export,omitempty"`
	IsComment    bool `json:"is_comment"`
	IsEmpty      bool `json:"is_empty"`
}

// IsAssignment reports whether the entry defines a key.
func (e *EnvEntry) IsAssignment() bool {
	return e.Key != ""
}

// EnvFile represents the complete abstract syntax tree of an environment file.
type EnvFile struct {
	Path    string     `json:"path"`
	Entries []EnvEntry `json:"entries"`
	// Index maps each key to its last occurrence in Entries. Its pointers
	// refer into Entries, so call Reindex after modifying the slice.
	Index      map[string]*EnvEntry `json:"-"`
	RawContent []byte               `json:"-"`
}

// Reindex rebuilds Index from Entries. When a key appears more than once,
// the last occurrence wins, matching shell semantics.
func (f *EnvFile) Reindex() {
	f.Index = make(map[string]*EnvEntry, len(f.Entries))
	for i := range f.Entries {
		if f.Entries[i].IsAssignment() {
			f.Index[f.Entries[i].Key] = &f.Entries[i]
		}
	}
}

// Get returns the effective (last) entry for key.
func (f *EnvFile) Get(key string) (*EnvEntry, bool) {
	e, ok := f.Index[key]
	return e, ok
}

// Keys returns each defined key once, in order of first appearance.
func (f *EnvFile) Keys() []string {
	seen := make(map[string]struct{}, len(f.Entries))
	keys := make([]string, 0, len(f.Entries))
	for i := range f.Entries {
		k := f.Entries[i].Key
		if k == "" {
			continue
		}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	return keys
}
