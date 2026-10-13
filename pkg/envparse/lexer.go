package envparse

import (
	"fmt"
	"unicode/utf8"
)

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokNewline
	tokExport
	tokKey
	tokAssign
	tokValue
	tokComment
)

func (k tokenKind) String() string {
	switch k {
	case tokEOF:
		return "EOF"
	case tokNewline:
		return "NEWLINE"
	case tokExport:
		return "EXPORT"
	case tokKey:
		return "KEY"
	case tokAssign:
		return "ASSIGN"
	case tokValue:
		return "VALUE"
	case tokComment:
		return "COMMENT"
	default:
		return fmt.Sprintf("tokenKind(%d)", int(k))
	}
}

type token struct {
	kind tokenKind
	// text is the raw source text of the token. For values it includes
	// the surrounding quotes.
	text string
	// value is the decoded value (tokValue only).
	value   string
	quote   QuoteType
	line    int
	col     int
	endLine int
}

const eof rune = -1

// stateFn is one state of the lexer's finite-state machine. It consumes
// input, emits tokens, and returns the next state (nil to stop).
type stateFn func(*lexer) stateFn

type lexer struct {
	path   string
	input  string
	pos    int // byte offset of the next rune
	line   int // 1-based line of the next rune
	col    int // 1-based column (in runes) of the next rune
	tokens []token
	err    *ParseError

	// Per-line state.
	sawExport bool
	// Start of the token currently being scanned.
	startPos  int
	startLine int
	startCol  int
}

// lex tokenizes input, which must already have its BOM stripped and line
// endings normalized to "\n".
func lex(path, input string) ([]token, error) {
	l := &lexer{
		path:   path,
		input:  input,
		line:   1,
		col:    1,
		tokens: make([]token, 0, len(input)/8+4),
	}
	for state := lexLineStart; state != nil; {
		state = state(l)
	}
	if l.err != nil {
		return nil, l.err
	}
	return l.tokens, nil
}

func (l *lexer) peek() rune {
	if l.pos >= len(l.input) {
		return eof
	}
	r, _ := utf8.DecodeRuneInString(l.input[l.pos:])
	return r
}

func (l *lexer) next() rune {
	if l.pos >= len(l.input) {
		return eof
	}
	r, w := utf8.DecodeRuneInString(l.input[l.pos:])
	l.pos += w
	if r == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return r
}

func (l *lexer) skipHorizontalSpace() int {
	n := 0
	for isHorizontalSpace(l.peek()) {
		l.next()
		n++
	}
	return n
}

func (l *lexer) mark() {
	l.startPos, l.startLine, l.startCol = l.pos, l.line, l.col
}

func (l *lexer) emit(kind tokenKind) {
	l.tokens = append(l.tokens, token{
		kind:    kind,
		text:    l.input[l.startPos:l.pos],
		line:    l.startLine,
		col:     l.startCol,
		endLine: l.line,
	})
}

func (l *lexer) emitValue(raw, value string, quote QuoteType) {
	l.tokens = append(l.tokens, token{
		kind:    tokValue,
		text:    raw,
		value:   value,
		quote:   quote,
		line:    l.startLine,
		col:     l.startCol,
		endLine: l.line,
	})
}

func (l *lexer) errorAt(line, col int, format string, args ...any) stateFn {
	l.err = &ParseError{Path: l.path, Line: line, Col: col, Msg: fmt.Sprintf(format, args...)}
	return nil
}

func (l *lexer) errorf(format string, args ...any) stateFn {
	return l.errorAt(l.line, l.col, format, args...)
}

// lexLineStart handles the beginning of a logical line: blank lines,
// full-line comments, and the optional `export` prefix.
func lexLineStart(l *lexer) stateFn {
	l.sawExport = false
	l.skipHorizontalSpace()
	switch r := l.peek(); {
	case r == eof:
		l.mark()
		l.emit(tokEOF)
		return nil
	case r == '\n':
		return lexNewline
	case r == '#':
		return lexComment
	}
	if l.atExportKeyword() {
		l.mark()
		for range len("export") {
			l.next()
		}
		l.emit(tokExport)
		l.sawExport = true
		l.skipHorizontalSpace()
	}
	return lexKey
}

// atExportKeyword reports whether the input at the current position is the
// `export` keyword rather than a key that happens to be named "export"
// (as in `export=1` or `export = 1`).
func (l *lexer) atExportKeyword() bool {
	const kw = "export"
	rest := l.input[l.pos:]
	if len(rest) <= len(kw) || rest[:len(kw)] != kw || !isHorizontalSpace(rune(rest[len(kw)])) {
		return false
	}
	i := len(kw)
	for i < len(rest) && isHorizontalSpace(rune(rest[i])) {
		i++
	}
	return i < len(rest) && rest[i] != '=' && rest[i] != '\n'
}

func lexNewline(l *lexer) stateFn {
	l.mark()
	l.next()
	l.emit(tokNewline)
	return lexLineStart
}

func lexComment(l *lexer) stateFn {
	l.mark()
	for r := l.peek(); r != '\n' && r != eof; r = l.peek() {
		l.next()
	}
	l.emit(tokComment)
	return lexLineStart
}

func lexKey(l *lexer) stateFn {
	l.mark()
	r := l.peek()
	if r == '=' {
		return l.errorf("missing key before '='")
	}
	if !isKeyStart(r) {
		return l.errorf("invalid character %q at start of key", r)
	}
	for isKeyChar(l.peek()) {
		l.next()
	}
	l.emit(tokKey)
	return lexPreAssign
}

func lexPreAssign(l *lexer) stateFn {
	spaces := l.skipHorizontalSpace()
	switch r := l.peek(); {
	case r == '=':
		l.mark()
		l.next()
		l.emit(tokAssign)
		return lexValueStart
	case l.sawExport && (r == '\n' || r == eof):
		return lexLineStart
	case l.sawExport && r == '#' && spaces > 0:
		return lexComment
	case r == '\n' || r == eof:
		return l.errorf("expected '=' after key")
	case spaces == 0:
		return l.errorf("invalid character %q in key", r)
	default:
		return l.errorf("unexpected %q after key; expected '='", r)
	}
}

// lexValueStart dispatches on the first non-space rune of a value. As in
// docker compose and godotenv, `KEY= #c` is an empty value with a comment
// while `KEY=#ff0000` is the literal value "#ff0000".
func lexValueStart(l *lexer) stateFn {
	spaces := l.skipHorizontalSpace()
	l.mark()
	switch r := l.peek(); {
	case r == '\n' || r == eof:
		l.emitValue("", "", QuoteNone)
		return lexLineStart
	case r == '#' && spaces > 0:
		l.emitValue("", "", QuoteNone)
		return lexComment
	case r == '\'':
		return lexSingleQuoted
	case r == '"':
		return lexDoubleQuoted
	case r == '`':
		return lexBacktickQuoted
	default:
		return lexUnquoted
	}
}

// lexUnquoted scans to end of line. A '#' preceded by whitespace starts an
// inline comment; elsewhere it is literal (as in `a#b`). Trailing
// whitespace is not part of the value.
func lexUnquoted(l *lexer) stateFn {
	end := l.pos
	endLine := l.line
	prevSpace := false
	for {
		r := l.peek()
		if r == '\n' || r == eof || (r == '#' && prevSpace) {
			break
		}
		l.next()
		prevSpace = isHorizontalSpace(r)
		if !prevSpace {
			end = l.pos
			endLine = l.line
		}
	}
	raw := l.input[l.startPos:end]
	l.tokens = append(l.tokens, token{
		kind:    tokValue,
		text:    raw,
		value:   raw,
		quote:   QuoteNone,
		line:    l.startLine,
		col:     l.startCol,
		endLine: endLine,
	})
	if l.peek() == '#' {
		return lexComment
	}
	return lexLineStart
}

func lexSingleQuoted(l *lexer) stateFn { return lexLiteralQuoted(l, QuoteSingle) }

func lexBacktickQuoted(l *lexer) stateFn { return lexLiteralQuoted(l, QuoteBacktick) }

// lexLiteralQuoted scans a single-quoted or backtick value. Contents are
// taken verbatim (no escapes) and may span multiple lines.
func lexLiteralQuoted(l *lexer, q QuoteType) stateFn {
	l.next() // opening quote
	for {
		switch l.next() {
		case eof:
			return l.errorAt(l.startLine, l.startCol, "unterminated %s-quoted value", q)
		case rune(q):
			raw := l.input[l.startPos:l.pos]
			l.emitValue(raw, raw[1:len(raw)-1], q)
			return lexPostQuote
		}
	}
}

// lexDoubleQuoted scans a double-quoted value, which may span multiple
// lines. A backslash escapes the following rune; escapes are decoded by
// render.
func lexDoubleQuoted(l *lexer) stateFn {
	l.next() // opening quote
	for {
		switch l.next() {
		case eof:
			return l.errorAt(l.startLine, l.startCol, "unterminated double-quoted value")
		case '\\':
			if l.next() == eof {
				return l.errorAt(l.startLine, l.startCol, "unterminated double-quoted value")
			}
		case '"':
			raw := l.input[l.startPos:l.pos]
			value, _ := render(raw[1:len(raw)-1], QuoteDouble, nil)
			l.emitValue(raw, value, QuoteDouble)
			return lexPostQuote
		}
	}
}

// lexPostQuote allows only whitespace or a comment after a closing quote.
func lexPostQuote(l *lexer) stateFn {
	l.skipHorizontalSpace()
	switch r := l.peek(); r {
	case '\n', eof:
		return lexLineStart
	case '#':
		return lexComment
	default:
		return l.errorf("unexpected %q after closing quote", r)
	}
}

func isHorizontalSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\v' || r == '\f' || r == '\r'
}

func isKeyStart(r rune) bool {
	return r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

func isKeyChar(r rune) bool {
	return isKeyStart(r) || (r >= '0' && r <= '9') || r == '.'
}
