package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type result struct {
	code           int
	stdout, stderr string
}

func runCLI(t *testing.T, args ...string) result {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, strings.NewReader(""), &out, &errOut)
	return result{code, out.String(), errOut.String()}
}

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const (
	srcEnv = `PORT=8080
DATABASE_URL=postgres://localhost/app
DEBUG=true
SECRET=sk_test_123
`
	tplEnv = `PORT=3000
DATABASE_URL=
DEBUG=sometimes
LOG_LEVEL=debug
`
)

func TestDiffIdentical(t *testing.T) {
	dir := writeFiles(t, map[string]string{"a.env": "A=1\nB=2\n", "b.env": "B=2\nA='1'\n"})
	r := runCLI(t, "diff", filepath.Join(dir, "a.env"), filepath.Join(dir, "b.env"))
	if r.code != ExitOK || !strings.Contains(r.stdout, "No differences") || !strings.Contains(r.stdout, "(2 keys)") {
		t.Errorf("got %+v", r)
	}
}

func TestDiffText(t *testing.T) {
	dir := writeFiles(t, map[string]string{".env": srcEnv, ".env.example": tplEnv})
	src, tpl := filepath.Join(dir, ".env"), filepath.Join(dir, ".env.example")
	r := runCLI(t, "diff", src, tpl)
	if r.code != ExitDrift {
		t.Fatalf("exit code = %d, want %d\n%s%s", r.code, ExitDrift, r.stdout, r.stderr)
	}
	for _, want := range []string{
		"--- " + src, "+++ " + tpl,
		"~ PORT", "value differs",
		"≠ DEBUG", "type mismatch (bool vs string)",
		"- SECRET", "missing in " + tpl,
		"+ LOG_LEVEL", "missing in " + src,
		"5 differences: 2 changed, 1 type mismatch, 1 missing in " + tpl + ", 1 missing in " + src,
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q:\n%s", want, r.stdout)
		}
	}
	if strings.Contains(r.stdout, "sk_test_123") || strings.Contains(r.stdout, "postgres://localhost") {
		t.Errorf("values leaked without --show-values:\n%s", r.stdout)
	}
	if r.stderr != "" {
		t.Errorf("drift should exit silently, stderr = %q", r.stderr)
	}
}

func TestDiffShowValues(t *testing.T) {
	dir := writeFiles(t, map[string]string{".env": srcEnv, ".env.example": tplEnv})
	r := runCLI(t, "diff", "--show-values", filepath.Join(dir, ".env"), filepath.Join(dir, ".env.example"))
	for _, want := range []string{`"8080" → "3000"`, `"sk_test_123" → (missing)`, `(missing) → "debug"`} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q:\n%s", want, r.stdout)
		}
	}
}

func TestDiffKeysOnly(t *testing.T) {
	dir := writeFiles(t, map[string]string{".env": srcEnv, ".env.example": tplEnv})
	r := runCLI(t, "diff", "-k", filepath.Join(dir, ".env"), filepath.Join(dir, ".env.example"))
	if r.code != ExitDrift || strings.Contains(r.stdout, "PORT") || !strings.Contains(r.stdout, "2 differences") {
		t.Errorf("got %+v", r)
	}

	dir = writeFiles(t, map[string]string{"a": "A=1\n", "b": "A=\n"})
	if r := runCLI(t, "diff", "--keys-only", filepath.Join(dir, "a"), filepath.Join(dir, "b")); r.code != ExitOK {
		t.Errorf("keys-only with same keys: %+v", r)
	}
}

func TestDiffJSON(t *testing.T) {
	dir := writeFiles(t, map[string]string{".env": srcEnv, ".env.example": tplEnv})
	src, tpl := filepath.Join(dir, ".env"), filepath.Join(dir, ".env.example")

	r := runCLI(t, "diff", "--json", src, tpl)
	if r.code != ExitDrift {
		t.Fatalf("exit code = %d", r.code)
	}
	var out jsonDiff
	if err := json.Unmarshal([]byte(r.stdout), &out); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, r.stdout)
	}
	if !out.HasDrift || out.Source != src || len(out.Results) != 5 || out.Summary.Differences() != 5 {
		t.Errorf("unexpected JSON: %+v", out)
	}
	if strings.Contains(r.stdout, "sk_test_123") || strings.Contains(r.stdout, `"value"`) {
		t.Errorf("values leaked without --show-values:\n%s", r.stdout)
	}
	byKey := map[string]jsonResult{}
	for _, res := range out.Results {
		byKey[res.Key] = res
	}
	if s := byKey["SECRET"]; s.Target != nil || s.Source == nil || s.Source.Line != 4 {
		t.Errorf("SECRET = %+v", s)
	}

	r = runCLI(t, "diff", "--json", "--show-values", src, tpl)
	if !strings.Contains(r.stdout, `"value": "sk_test_123"`) || !strings.Contains(r.stdout, `"value": ""`) {
		t.Errorf("--show-values JSON missing values:\n%s", r.stdout)
	}
}

func TestDiffErrors(t *testing.T) {
	dir := writeFiles(t, map[string]string{"ok.env": "A=1\n", "bad.env": "A=1\nB=\"unterminated\n"})
	ok, bad := filepath.Join(dir, "ok.env"), filepath.Join(dir, "bad.env")
	tests := []struct {
		name   string
		args   []string
		stderr string
	}{
		{"missing file", []string{"diff", ok, filepath.Join(dir, "nope.env")}, "nope.env"},
		{"parse error", []string{"diff", ok, bad}, "bad.env:2:3: unterminated double-quoted value"},
		{"one arg", []string{"diff", ok}, "accepts 2 arg(s)"},
		{"interactive without terminal", []string{"diff", "-i", ok, ok}, "--interactive requires a terminal"},
		{"conflicting flags", []string{"diff", "-i", "--json", ok, ok}, "none of the others can be"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := runCLI(t, tt.args...)
			if r.code != ExitError || !strings.Contains(r.stderr, tt.stderr) {
				t.Errorf("got code %d stderr %q, want %d containing %q", r.code, r.stderr, ExitError, tt.stderr)
			}
		})
	}
}

func TestParseCommand(t *testing.T) {
	dir := writeFiles(t, map[string]string{".env": "export A=1 # c\nB=\"${A}x\"\n"})
	path := filepath.Join(dir, ".env")

	r := runCLI(t, "parse", path)
	if r.code != ExitOK || !strings.Contains(r.stdout, `A="1"  [exported, comment="# c"]`) {
		t.Errorf("text output: %+v", r)
	}
	r = runCLI(t, "parse", "--expand", path)
	if !strings.Contains(r.stdout, `B="1x"`) {
		t.Errorf("expanded output: %s", r.stdout)
	}
	r = runCLI(t, "parse", "--json", path)
	if !strings.Contains(r.stdout, `"quote_type": "double"`) {
		t.Errorf("json output: %s", r.stdout)
	}
}

func TestVersion(t *testing.T) {
	r := runCLI(t, "--version")
	if r.code != ExitOK || !strings.HasPrefix(r.stdout, "envy version ") {
		t.Errorf("got %+v", r)
	}
}
