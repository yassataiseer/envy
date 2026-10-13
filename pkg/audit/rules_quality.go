package audit

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yassataiseer/envy/pkg/envparse"
)

const (
	RuleDuplicateKey       = "QLT001"
	RuleTrailingWhitespace = "QLT002"
	RuleUnquotedComplex    = "QLT003"
	RuleInconsistentExport = "QLT004"
)

// duplicateKeyRule (QLT001) flags keys defined more than once; only the
// last definition takes effect.
type duplicateKeyRule struct{}

func (duplicateKeyRule) ID() string { return RuleDuplicateKey }

func (duplicateKeyRule) Evaluate(f *envparse.EnvFile) []AuditFinding {
	lines := map[string][]int{}
	for i := range f.Entries {
		if e := &f.Entries[i]; e.IsAssignment() {
			lines[e.Key] = append(lines[e.Key], e.LineNumber)
		}
	}
	var out []AuditFinding
	for _, key := range f.Keys() {
		ls := lines[key]
		if len(ls) < 2 {
			continue
		}
		last := ls[len(ls)-1]
		for _, l := range ls[:len(ls)-1] {
			out = append(out, AuditFinding{
				RuleID:      RuleDuplicateKey,
				Rule:        "Duplicate Key",
				Severity:    SeverityWarning,
				Key:         key,
				LineNumber:  l,
				Message:     fmt.Sprintf("KEY '%s' is defined %d times; this definition is overridden by line %d.", key, len(ls), last),
				Remediation: "Remove the duplicate definitions so the effective value is unambiguous.",
			})
		}
	}
	return out
}

// trailingWhitespaceRule (QLT002) flags lines ending in spaces or tabs,
// which are silently dropped from unquoted values.
type trailingWhitespaceRule struct{}

func (trailingWhitespaceRule) ID() string { return RuleTrailingWhitespace }

func (trailingWhitespaceRule) Evaluate(f *envparse.EnvFile) []AuditFinding {
	src := strings.ReplaceAll(strings.TrimPrefix(string(f.RawContent), "\xEF\xBB\xBF"), "\r\n", "\n")
	lines := strings.Split(src, "\n")
	var out []AuditFinding
	for i := range f.Entries {
		e := &f.Entries[i]
		if e.EndLine < 1 || e.EndLine > len(lines) {
			continue
		}
		line := lines[e.EndLine-1]
		if !strings.HasSuffix(line, " ") && !strings.HasSuffix(line, "\t") {
			continue
		}
		out = append(out, AuditFinding{
			RuleID:      RuleTrailingWhitespace,
			Rule:        "Trailing Whitespace",
			Severity:    SeverityInfo,
			Key:         e.Key,
			LineNumber:  e.EndLine,
			Message:     "Line " + strconv.Itoa(e.EndLine) + " ends with whitespace, which is easy to miss and is dropped from unquoted values.",
			Remediation: "Remove the trailing whitespace, or quote the value if the spaces are intentional.",
		})
	}
	return out
}

// unquotedComplexRule (QLT003) flags unquoted values that a shell would
// split or interpret when the file is sourced.
type unquotedComplexRule struct{}

func (unquotedComplexRule) ID() string { return RuleUnquotedComplex }

func (unquotedComplexRule) Evaluate(f *envparse.EnvFile) []AuditFinding {
	var out []AuditFinding
	for _, e := range assignments(f) {
		if e.QuoteType != envparse.QuoteNone || !strings.ContainsAny(e.Value, " \t'\"`;&|<>()") {
			continue
		}
		out = append(out, AuditFinding{
			RuleID:      RuleUnquotedComplex,
			Rule:        "Unquoted Complex Value",
			Severity:    SeverityInfo,
			Key:         e.Key,
			LineNumber:  e.LineNumber,
			Message:     fmt.Sprintf("KEY '%s' has an unquoted value containing spaces or shell metacharacters; it will break if the file is sourced by a shell.", e.Key),
			Remediation: "Wrap the value in double quotes.",
		})
	}
	return out
}

// inconsistentExportRule (QLT004) flags files that use `export` on some
// keys but not others.
type inconsistentExportRule struct{}

func (inconsistentExportRule) ID() string { return RuleInconsistentExport }

func (inconsistentExportRule) Evaluate(f *envparse.EnvFile) []AuditFinding {
	var exported, total int
	var first *envparse.EnvEntry
	for i := range f.Entries {
		e := &f.Entries[i]
		if !e.IsAssignment() {
			continue
		}
		total++
		if e.IsExported {
			exported++
		} else if first == nil {
			first = e
		}
	}
	if exported == 0 || exported == total {
		return nil
	}
	return []AuditFinding{{
		RuleID:      RuleInconsistentExport,
		Rule:        "Missing Export Directive",
		Severity:    SeverityInfo,
		Key:         first.Key,
		LineNumber:  first.LineNumber,
		Message:     fmt.Sprintf("%d of %d keys use 'export'; KEY '%s' and %d others do not, so sourcing this file exports only some variables.", exported, total, first.Key, total-exported-1),
		Remediation: "Use 'export' on every key or on none.",
	}}
}
