package envparse

import "fmt"

// ParseError reports a syntax error at a 1-based line and column.
// Columns count runes, not bytes.
type ParseError struct {
	Path string
	Line int
	Col  int
	Msg  string
}

func (e *ParseError) Error() string {
	path := e.Path
	if path == "" {
		path = "<input>"
	}
	return fmt.Sprintf("%s:%d:%d: %s", path, e.Line, e.Col, e.Msg)
}

// ExpandError reports a failure while expanding variable references.
type ExpandError struct {
	Path string
	Key  string
	Line int
	Msg  string
}

func (e *ExpandError) Error() string {
	path := e.Path
	if path == "" {
		path = "<input>"
	}
	return fmt.Sprintf("%s:%d: expanding %s: %s", path, e.Line, e.Key, e.Msg)
}
