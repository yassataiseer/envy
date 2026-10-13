// Package sanitize turns a real .env file into a template that is safe to
// commit, keeping its keys, comments, and layout.
package sanitize

import (
	"strings"

	"github.com/yassataiseer/envy/pkg/audit"
	"github.com/yassataiseer/envy/pkg/diff"
	"github.com/yassataiseer/envy/pkg/envparse"
)

type Options struct {
	// Mask replaces values with a type placeholder such as <secret> or
	// <url> instead of the empty string.
	Mask bool
	// KeepSafe keeps values that do not look sensitive, such as PORT=3000.
	KeepSafe bool
}

// Result counts what Sanitize changed.
type Result struct {
	Stripped int
	Kept     int
	// Comments counts commented-out assignments whose values were removed.
	Comments int
}

// Sanitize strips values from f in place. Every occurrence of a key is
// handled, including overridden duplicates, and so are commented-out
// assignments such as `# STRIPE_KEY=sk_live_...`.
func Sanitize(f *envparse.EnvFile, opts Options) Result {
	var res Result
	for i := range f.Entries {
		e := &f.Entries[i]
		switch {
		case e.IsComment:
			if text, ok := sanitizeComment(e.RawComment, opts); ok {
				_ = f.SetCommentAt(i, text)
				res.Comments++
			}
		case !e.IsAssignment() || e.IsBareExport || e.Value == "":
		case opts.KeepSafe && !audit.IsSensitive(e) && e.EndLine == e.LineNumber:
			res.Kept++
		default:
			_ = f.SetAt(i, replacement(e, opts))
			res.Stripped++
		}
	}
	return res
}

func replacement(e *envparse.EnvEntry, opts Options) string {
	if !opts.Mask {
		return ""
	}
	return Placeholder(e)
}

// Placeholder describes an entry's value without revealing it.
func Placeholder(e *envparse.EnvEntry) string {
	if audit.IsSensitive(e) {
		return "<secret>"
	}
	switch diff.InferKind(e.Value) {
	case diff.KindURL:
		return "<url>"
	case diff.KindInt, diff.KindFloat:
		return "<number>"
	case diff.KindBool:
		return "<true|false>"
	}
	return "<value>"
}

// sanitizeComment strips the value from a comment that contains a single
// commented-out assignment, such as "# API_KEY=abc # old key".
func sanitizeComment(comment string, opts Options) (string, bool) {
	body := strings.TrimLeft(strings.TrimPrefix(comment, "#"), " \t")
	parsed, err := envparse.ParseBytes([]byte(body), "")
	if err != nil || len(parsed.Entries) != 1 || !parsed.Entries[0].IsAssignment() {
		return "", false
	}
	inner := &parsed.Entries[0]
	if inner.IsBareExport || inner.Value == "" || (opts.KeepSafe && !audit.IsSensitive(inner)) {
		return "", false
	}
	_ = parsed.SetAt(0, replacement(inner, opts))
	prefix := comment[:len(comment)-len(body)]
	return prefix + strings.TrimRight(string(parsed.Format()), "\n"), true
}
