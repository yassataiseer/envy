// Package audit runs static security and quality checks against parsed
// .env files. Findings never include secret values.
package audit

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/yassataiseer/envy/pkg/envparse"
)

type Severity string

const (
	SeverityInfo     Severity = "INFO"
	SeverityWarning  Severity = "WARNING"
	SeverityCritical Severity = "CRITICAL"
)

// Rank orders severities: Info < Warning < Critical.
func (s Severity) Rank() int {
	switch s {
	case SeverityCritical:
		return 3
	case SeverityWarning:
		return 2
	case SeverityInfo:
		return 1
	}
	return 0
}

// AuditFinding details a single security or compliance violation.
type AuditFinding struct {
	RuleID      string   `json:"rule_id"`
	Rule        string   `json:"rule"`
	Severity    Severity `json:"severity"`
	Key         string   `json:"key,omitempty"`
	LineNumber  int      `json:"line_number"`
	Message     string   `json:"message"`
	Remediation string   `json:"remediation"`
}

// Auditor defines the interface for static analysis rules.
type Auditor interface {
	ID() string
	Evaluate(file *envparse.EnvFile) []AuditFinding
}

// Options tunes the default rule set.
type Options struct {
	// EntropyThreshold is the Shannon entropy (bits per character) above
	// which SEC002 flags a value. Hex-only values use HexEntropyThreshold.
	EntropyThreshold    float64
	HexEntropyThreshold float64
}

// DefaultOptions returns the thresholds from the design document.
func DefaultOptions() Options {
	return Options{EntropyThreshold: 4.5, HexEntropyThreshold: 3.0}
}

// Rules returns the default rule set.
func Rules(opts Options) []Auditor {
	return []Auditor{
		prefixLeakRule{},
		entropyRule{threshold: opts.EntropyThreshold, hexThreshold: opts.HexEntropyThreshold},
		knownFormatRule{},
		urlCredentialsRule{},
		duplicateKeyRule{},
		trailingWhitespaceRule{},
		unquotedComplexRule{},
		inconsistentExportRule{},
	}
}

// Audit runs the default rules against f.
func Audit(f *envparse.EnvFile, opts Options) []AuditFinding {
	return Run(f, Rules(opts))
}

// Run evaluates rules against f, drops findings suppressed with an
// `# envy:ignore [RULE...]` inline comment, removes entropy findings that
// a more specific rule already explains, and sorts the result by severity
// (most severe first) and then line.
func Run(f *envparse.EnvFile, rules []Auditor) []AuditFinding {
	ignores := collectIgnores(f)
	var all []AuditFinding
	for _, r := range rules {
		for _, fd := range r.Evaluate(f) {
			if ignores.suppressed(fd) {
				continue
			}
			all = append(all, fd)
		}
	}
	all = dropRedundantEntropy(all)
	slices.SortStableFunc(all, func(a, b AuditFinding) int {
		if d := b.Severity.Rank() - a.Severity.Rank(); d != 0 {
			return d
		}
		return a.LineNumber - b.LineNumber
	})
	return all
}

// dropRedundantEntropy removes SEC002 findings for keys that SEC001 or
// SEC003 already flag, since those explain the problem more precisely.
func dropRedundantEntropy(fs []AuditFinding) []AuditFinding {
	explained := make(map[int]bool)
	for _, f := range fs {
		if f.RuleID == RuleKnownFormat || f.RuleID == RulePrefixLeak {
			explained[f.LineNumber] = true
		}
	}
	out := fs[:0]
	for _, f := range fs {
		if f.RuleID == RuleEntropy && explained[f.LineNumber] {
			continue
		}
		out = append(out, f)
	}
	return out
}

// ignoreSet maps a line number to the rule IDs ignored on it. An empty,
// non-nil slice means every rule is ignored on that line.
type ignoreSet map[int][]string

const ignoreDirective = "envy:ignore"

func collectIgnores(f *envparse.EnvFile) ignoreSet {
	set := ignoreSet{}
	for i := range f.Entries {
		e := &f.Entries[i]
		if !e.IsAssignment() {
			continue
		}
		idx := strings.Index(e.RawComment, ignoreDirective)
		if idx < 0 {
			continue
		}
		ids := strings.FieldsFunc(e.RawComment[idx+len(ignoreDirective):], func(r rune) bool {
			return r == ' ' || r == ',' || r == '\t'
		})
		for j := range ids {
			ids[j] = strings.ToUpper(ids[j])
		}
		if ids == nil {
			ids = []string{}
		}
		set[e.LineNumber] = ids
		set[e.EndLine] = ids
	}
	return set
}

func (s ignoreSet) suppressed(f AuditFinding) bool {
	ids, ok := s[f.LineNumber]
	if !ok {
		return false
	}
	return len(ids) == 0 || slices.Contains(ids, f.RuleID)
}

// Summary counts findings by severity.
type Summary struct {
	Critical int `json:"critical"`
	Warning  int `json:"warning"`
	Info     int `json:"info"`
}

func Summarize(fs []AuditFinding) Summary {
	var s Summary
	for _, f := range fs {
		switch f.Severity {
		case SeverityCritical:
			s.Critical++
		case SeverityWarning:
			s.Warning++
		case SeverityInfo:
			s.Info++
		}
	}
	return s
}

// IsTemplatePath reports whether path names a template that is normally
// committed, such as .env.example, .env.sample, or example.env.
func IsTemplatePath(path string) bool {
	for _, part := range strings.Split(strings.ToLower(filepath.Base(path)), ".") {
		switch part {
		case "example", "sample", "template", "dist", "defaults":
			return true
		}
	}
	return false
}
