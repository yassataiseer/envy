package envparse

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// Parse reads all of r and parses it as a .env file. path is used only for
// error messages and EnvFile.Path.
func Parse(r io.Reader, path string) (*EnvFile, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("envparse: reading %s: %w", path, err)
	}
	return ParseBytes(b, path)
}

// ParseFile reads and parses the .env file at path.
func ParseFile(path string) (*EnvFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseBytes(b, path)
}

// ParseBytes parses b as a .env file. b is retained as EnvFile.RawContent
// and must not be modified afterwards.
func ParseBytes(b []byte, path string) (*EnvFile, error) {
	tokens, err := lex(path, normalize(b))
	if err != nil {
		return nil, err
	}
	f := &EnvFile{
		Path:       path,
		Entries:    buildEntries(tokens),
		RawContent: b,
	}
	f.Reindex()
	return f, nil
}

// normalize strips a UTF-8 byte order mark and converts CRLF line endings
// to LF.
func normalize(b []byte) string {
	b = bytes.TrimPrefix(b, utf8BOM)
	if bytes.IndexByte(b, '\r') < 0 {
		return string(b)
	}
	return string(bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")))
}

// buildEntries groups the token stream into one entry per logical line.
// The lexer guarantees tokens arrive in a valid order, so no further
// syntax checking is needed here.
func buildEntries(tokens []token) []EnvEntry {
	entries := make([]EnvEntry, 0, len(tokens)/3+1)
	var cur EnvEntry
	started := false

	flush := func(lineOfTerminator int) {
		if !started {
			cur = EnvEntry{IsEmpty: true, LineNumber: lineOfTerminator, EndLine: lineOfTerminator}
		}
		entries = append(entries, cur)
		cur = EnvEntry{}
		started = false
	}

	for _, t := range tokens {
		switch t.kind {
		case tokExport:
			cur.IsExported = true
			cur.IsBareExport = true
			cur.LineNumber, cur.EndLine = t.line, t.line
			started = true
		case tokKey:
			cur.Key = t.text
			if !started {
				cur.LineNumber, cur.EndLine = t.line, t.line
			}
			started = true
		case tokAssign:
			cur.IsBareExport = false
		case tokValue:
			cur.Value = t.value
			cur.RawValue = t.text
			cur.QuoteType = t.quote
			cur.EndLine = t.endLine
		case tokComment:
			cur.RawComment = t.text
			if !started {
				cur.IsComment = true
				cur.LineNumber, cur.EndLine = t.line, t.line
				started = true
			}
		case tokNewline:
			flush(t.line)
		case tokEOF:
			if started {
				flush(t.line)
			}
		}
	}
	return entries
}
