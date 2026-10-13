package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

const leakyEnv = `PORT=3000
NEXT_PUBLIC_STRIPE_SECRET=sk_123
JWT_SIGNING_KEY=q8F2zR9vL4mX7wT1nB6kP3jH5yC0
GREETING=hello world
`

func TestAuditText(t *testing.T) {
	dir := writeFiles(t, map[string]string{".env": leakyEnv})
	r := runCLI(t, "audit", filepath.Join(dir, ".env"))
	if r.code != ExitDrift {
		t.Fatalf("exit = %d, want %d\n%s%s", r.code, ExitDrift, r.stdout, r.stderr)
	}
	for _, want := range []string{
		"[CRITICAL] Rule SEC001 (Framework Prefix Violation)",
		"Line 2: KEY 'NEXT_PUBLIC_STRIPE_SECRET' contains framework prefix 'NEXT_PUBLIC_'",
		"Remediation: Rename the key to 'STRIPE_SECRET'",
		"[WARNING] Rule SEC002 (High Entropy Value Detected)",
		"Audit complete. 1 Critical, 1 Warning found.",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q:\n%s", want, r.stdout)
		}
	}
	if strings.Contains(r.stdout, "QLT003") {
		t.Error("INFO findings shown without --strict")
	}
	if strings.Contains(r.stdout, "q8F2zR9v") || strings.Contains(r.stdout, "sk_123") {
		t.Error("audit output contains a secret value")
	}
	if strings.Index(r.stdout, "CRITICAL") > strings.Index(r.stdout, "WARNING") {
		t.Error("critical findings should be listed first")
	}
}

func TestAuditExitCodes(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"clean.env": "PORT=3000\n",
		"warn.env":  "JWT_SIGNING_KEY=q8F2zR9vL4mX7wT1nB6kP3jH5yC0\n",
		"info.env":  "GREETING=hello world\n",
	})
	tests := []struct {
		file   string
		strict bool
		code   int
	}{
		{"clean.env", false, ExitOK},
		{"clean.env", true, ExitOK},
		{"warn.env", false, ExitOK},
		{"warn.env", true, ExitDrift},
		{"info.env", true, ExitOK},
	}
	for _, tt := range tests {
		args := []string{"audit", filepath.Join(dir, tt.file)}
		if tt.strict {
			args = append(args, "--strict")
		}
		if r := runCLI(t, args...); r.code != tt.code {
			t.Errorf("%s strict=%v: exit %d, want %d\n%s", tt.file, tt.strict, r.code, tt.code, r.stdout)
		}
	}
	r := runCLI(t, "audit", filepath.Join(dir, "clean.env"))
	if !strings.Contains(r.stdout, "Audit complete. No issues found.") {
		t.Errorf("clean output: %q", r.stdout)
	}
	r = runCLI(t, "audit", "--strict", filepath.Join(dir, "info.env"))
	if !strings.Contains(r.stdout, "[INFO] Rule QLT003") {
		t.Errorf("--strict should show INFO findings:\n%s", r.stdout)
	}
}

func TestAuditMultipleFilesAndJSON(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		".env":         leakyEnv,
		".env.example": "STRIPE_KEY=sk_test_4eC39HqL_yjWDarjt_T1zdp7dc\n",
	})
	env, example := filepath.Join(dir, ".env"), filepath.Join(dir, ".env.example")

	r := runCLI(t, "audit", env, example)
	if !strings.Contains(r.stdout, env+"\n") || !strings.Contains(r.stdout, example+"\n") || !strings.Contains(r.stdout, "2 Critical") {
		t.Errorf("multi-file output:\n%s", r.stdout)
	}

	r = runCLI(t, "audit", "--json", env, example)
	var out struct {
		Files []struct {
			Path     string `json:"path"`
			Findings []struct {
				RuleID   string `json:"rule_id"`
				Severity string `json:"severity"`
				Line     int    `json:"line_number"`
			} `json:"findings"`
		} `json:"files"`
		Summary struct{ Critical, Warning, Info int } `json:"summary"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &out); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, r.stdout)
	}
	if len(out.Files) != 2 || out.Summary.Critical != 2 || out.Files[1].Findings[0].RuleID != "SEC003" || out.Files[1].Findings[0].Severity != "CRITICAL" {
		t.Errorf("JSON = %+v", out)
	}
	if r.code != ExitDrift {
		t.Errorf("exit = %d", r.code)
	}
}

func TestAuditErrors(t *testing.T) {
	dir := writeFiles(t, map[string]string{"bad.env": "A='x\n"})
	if r := runCLI(t, "audit", filepath.Join(dir, "bad.env")); r.code != ExitError || !strings.Contains(r.stderr, "unterminated") {
		t.Errorf("got %+v", r)
	}
	if r := runCLI(t, "audit", filepath.Join(dir, "missing.env")); r.code != ExitError {
		t.Errorf("got %+v", r)
	}
}
