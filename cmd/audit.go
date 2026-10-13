package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/yassataiseer/envy/pkg/audit"
	"github.com/yassataiseer/envy/pkg/envparse"
)

type auditFlags struct {
	strict           bool
	asJSON           bool
	entropyThreshold float64
}

func newAuditCmd() *cobra.Command {
	var fl auditFlags
	c := &cobra.Command{
		Use:   "audit [path...]",
		Short: "Check .env files for leaked secrets and quality problems",
		Long: `Run static security and quality checks against .env files (default: .env).

Security rules:
  SEC001  secret behind a client-exposed prefix (NEXT_PUBLIC_, VITE_, ...)
  SEC002  high-entropy value that looks like a generated secret
  SEC003  recognizable credential (Stripe, GitHub, AWS, Slack, PEM keys, ...)
  SEC004  password embedded in a connection URL

Quality rules:
  QLT001  duplicate key            QLT003  unquoted value with spaces or shell metacharacters
  QLT002  trailing whitespace      QLT004  'export' used on some keys but not others

Secrets found in template files (.env.example, .env.sample, ...) are raised
to CRITICAL because those files are normally committed.

Silence a finding with an inline comment: KEY=value # envy:ignore SEC001

By default only CRITICAL findings fail the run and INFO findings are hidden.
--strict shows everything and also fails on WARNING.

Exit codes: 0 clean, 1 findings at the failing severity, 2 error.`,
		Example: `  envy audit
  envy audit .env .env.example --strict
  envy audit .env.production --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				args = []string{".env"}
			}
			return runAudit(cmd.OutOrStdout(), args, fl)
		},
	}
	f := c.Flags()
	f.BoolVar(&fl.strict, "strict", false, "show INFO findings and fail on WARNING as well as CRITICAL")
	f.BoolVar(&fl.asJSON, "json", false, "print findings as JSON")
	f.Float64Var(&fl.entropyThreshold, "entropy-threshold", audit.DefaultOptions().EntropyThreshold, "Shannon entropy (bits/char) above which values are flagged")
	return c
}

type fileReport struct {
	Path     string               `json:"path"`
	Findings []audit.AuditFinding `json:"findings"`
	Summary  audit.Summary        `json:"summary"`
}

func runAudit(out io.Writer, paths []string, fl auditFlags) error {
	opts := audit.DefaultOptions()
	opts.EntropyThreshold = fl.entropyThreshold

	reports := make([]fileReport, 0, len(paths))
	var total audit.Summary
	for _, p := range paths {
		f, err := envparse.ParseFile(p)
		if err != nil {
			return &exitError{Code: ExitError, Err: err}
		}
		var shown []audit.AuditFinding
		for _, fd := range audit.Audit(f, opts) {
			if fl.strict || fd.Severity != audit.SeverityInfo {
				shown = append(shown, fd)
			}
		}
		if shown == nil {
			shown = []audit.AuditFinding{}
		}
		s := audit.Summarize(shown)
		total.Critical += s.Critical
		total.Warning += s.Warning
		total.Info += s.Info
		reports = append(reports, fileReport{Path: p, Findings: shown, Summary: s})
	}

	var err error
	if fl.asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		err = enc.Encode(struct {
			Files   []fileReport  `json:"files"`
			Summary audit.Summary `json:"summary"`
		}{reports, total})
	} else {
		err = writeAuditText(out, reports, total)
	}
	if err != nil {
		return &exitError{Code: ExitError, Err: err}
	}

	if total.Critical > 0 || (fl.strict && total.Warning > 0) {
		return &exitError{Code: ExitDrift}
	}
	return nil
}

func writeAuditText(w io.Writer, reports []fileReport, total audit.Summary) error {
	multi := len(reports) > 1
	for _, r := range reports {
		if multi {
			fmt.Fprintf(w, "%s\n", r.Path)
		}
		indent := ""
		if multi {
			indent = "  "
		}
		for _, f := range r.Findings {
			fmt.Fprintf(w, "%s[%s] Rule %s (%s)\n", indent, f.Severity, f.RuleID, f.Rule)
			fmt.Fprintf(w, "%s  Line %d: %s\n", indent, f.LineNumber, f.Message)
			fmt.Fprintf(w, "%s  Remediation: %s\n\n", indent, f.Remediation)
		}
		if multi && len(r.Findings) == 0 {
			fmt.Fprintf(w, "%s  No findings.\n\n", indent)
		}
	}
	_, err := fmt.Fprintf(w, "Audit complete. %s found.\n", summaryText(total))
	return err
}

func summaryText(s audit.Summary) string {
	var parts []string
	add := func(n int, label string) {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, label))
		}
	}
	add(s.Critical, "Critical")
	add(s.Warning, plural(s.Warning, "Warning"))
	add(s.Info, "Info")
	if len(parts) == 0 {
		return "No issues"
	}
	return strings.Join(parts, ", ")
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
