package envparse

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// IsValidKey reports whether key is a legal .env key.
func IsValidKey(key string) bool {
	if key == "" || !isKeyStart(rune(key[0])) {
		return false
	}
	for i := 1; i < len(key); i++ {
		if !isKeyChar(rune(key[i])) {
			return false
		}
	}
	return true
}

// Set assigns value to key, quoting it as needed. If key is defined more
// than once, its last (effective) occurrence is updated; if it is not
// defined, a new entry is appended.
func (f *EnvFile) Set(key, value string) error {
	raw, q := QuoteValue(value)
	return f.assign(key, value, raw, q, false)
}

// SetFrom assigns src's value to src.Key exactly as written in src,
// keeping its quoting and any unexpanded ${VAR} references.
func (f *EnvFile) SetFrom(src *EnvEntry) error {
	if !src.IsAssignment() {
		return errors.New("envparse: SetFrom requires an assignment entry")
	}
	return f.assign(src.Key, src.Value, src.RawValue, src.QuoteType, src.IsBareExport)
}

func (f *EnvFile) assign(key, value, raw string, q QuoteType, bareExport bool) error {
	if !IsValidKey(key) {
		return fmt.Errorf("envparse: invalid key %q", key)
	}
	if e, ok := f.Index[key]; ok {
		e.Value, e.RawValue, e.QuoteType = value, raw, q
		e.IsBareExport = bareExport
		if bareExport {
			e.IsExported = true
		}
		e.modified = true
		return nil
	}
	f.Entries = append(f.Entries, EnvEntry{
		Key:          key,
		Value:        value,
		RawValue:     raw,
		QuoteType:    q,
		IsExported:   bareExport,
		IsBareExport: bareExport,
		modified:     true,
	})
	f.Reindex()
	return nil
}

// Delete removes every occurrence of key and reports whether any existed.
func (f *EnvFile) Delete(key string) bool {
	kept := f.Entries[:0]
	for _, e := range f.Entries {
		if e.Key != key {
			kept = append(kept, e)
		}
	}
	removed := len(kept) != len(f.Entries)
	clear(f.Entries[len(kept):])
	f.Entries = kept
	f.Reindex()
	return removed
}

// QuoteValue returns a source representation of value that parses back to
// exactly value. Simple values stay unquoted; anything containing
// whitespace, quotes, '#', '$', backslashes, or control characters is
// double-quoted with escapes, which keeps it on a single line.
func QuoteValue(value string) (raw string, q QuoteType) {
	if !needsQuoting(value) {
		return value, QuoteNone
	}
	var b strings.Builder
	b.Grow(len(value) + 2)
	b.WriteByte('"')
	for i := 0; i < len(value); i++ {
		switch c := value[i]; c {
		case '"', '\\', '$':
			b.WriteByte('\\')
			b.WriteByte(c)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String(), QuoteDouble
}

func needsQuoting(v string) bool {
	for i := 0; i < len(v); i++ {
		switch c := v[i]; {
		case c <= ' ' || c == 0x7f:
			return true
		case c == '"' || c == '\'' || c == '`' || c == '#' || c == '$' || c == '\\':
			return true
		}
	}
	return false
}

// Format renders the file back to bytes. Entries that have not been
// modified are copied verbatim from RawContent, so an unmodified file
// round-trips byte for byte (except that mixed line endings become
// uniformly CRLF if the file used CRLF anywhere). Modified and new entries
// are rendered in canonical form.
func (f *EnvFile) Format() []byte {
	src := normalize(f.RawContent)
	lines := strings.Split(src, "\n")

	parts := make([]string, 0, len(f.Entries))
	lastRendered := false
	for i := range f.Entries {
		e := &f.Entries[i]
		if !e.modified && e.LineNumber >= 1 && e.EndLine >= e.LineNumber && e.EndLine <= len(lines) {
			parts = append(parts, strings.Join(lines[e.LineNumber-1:e.EndLine], "\n"))
			lastRendered = false
		} else {
			parts = append(parts, renderEntry(e))
			lastRendered = true
		}
	}

	out := strings.Join(parts, "\n")
	if len(parts) > 0 && (lastRendered || strings.HasSuffix(src, "\n")) {
		out += "\n"
	}
	if bytes.Contains(f.RawContent, []byte("\r\n")) {
		out = strings.ReplaceAll(out, "\n", "\r\n")
	}
	if bytes.HasPrefix(f.RawContent, utf8BOM) {
		out = string(utf8BOM) + out
	}
	return []byte(out)
}

func renderEntry(e *EnvEntry) string {
	switch {
	case e.IsComment:
		return e.RawComment
	case e.IsEmpty:
		return ""
	}
	var b strings.Builder
	if e.IsExported {
		b.WriteString("export ")
	}
	b.WriteString(e.Key)
	if !e.IsBareExport {
		b.WriteByte('=')
		b.WriteString(e.RawValue)
	}
	if e.RawComment != "" {
		b.WriteByte(' ')
		b.WriteString(e.RawComment)
	}
	return b.String()
}

// WriteFile atomically writes Format() to path, preserving the permissions
// of an existing file (new files are created with mode 0600). On success,
// f is re-parsed from the written bytes so that line numbers and RawContent
// match the file on disk; pointers into the old Entries become stale.
func (f *EnvFile) WriteFile(path string) error {
	out := f.Format()
	reparsed, err := ParseBytes(out, path)
	if err != nil {
		return fmt.Errorf("envparse: refusing to write %s: rendered output does not parse: %w", path, err)
	}

	perm := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".envy-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	*f = *reparsed
	return nil
}
