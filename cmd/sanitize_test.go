package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const realEnv = "# config\nPORT=3000\nSTRIPE_SECRET_KEY=sk_test_4eC39HqL_yjWDarjt_T1zdp7dc\n"

func TestSanitizeToStdout(t *testing.T) {
	dir := writeFiles(t, map[string]string{".env": realEnv})
	r := runCLI(t, "sanitize", filepath.Join(dir, ".env"))
	if r.code != ExitOK || r.stdout != "# config\nPORT=\nSTRIPE_SECRET_KEY=\n" || r.stderr != "" {
		t.Errorf("got %+v", r)
	}
	r = runCLI(t, "sanitize", "--keep-safe", "--mask", filepath.Join(dir, ".env"))
	if r.stdout != "# config\nPORT=3000\nSTRIPE_SECRET_KEY=\"<secret>\"\n" {
		t.Errorf("keep-safe+mask stdout = %q", r.stdout)
	}
}

func TestSanitizeToFile(t *testing.T) {
	dir := writeFiles(t, map[string]string{".env": realEnv, ".env.example": "STALE=1\n"})
	in, out := filepath.Join(dir, ".env"), filepath.Join(dir, ".env.example")
	r := runCLI(t, "sanitize", in, "-o", out, "--keep-safe")
	if r.code != ExitOK || !strings.Contains(r.stderr, "1 value stripped, 1 kept") {
		t.Errorf("got %+v", r)
	}
	got, _ := os.ReadFile(out)
	if string(got) != "# config\nPORT=3000\nSTRIPE_SECRET_KEY=\n" {
		t.Errorf("template = %q", got)
	}
	if orig, _ := os.ReadFile(in); string(orig) != realEnv {
		t.Error("input was modified")
	}

	if a := runCLI(t, "audit", out); a.code != ExitOK {
		t.Errorf("sanitized template fails audit:\n%s", a.stdout)
	}
}

func TestSanitizeRefusesToOverwriteInput(t *testing.T) {
	dir := writeFiles(t, map[string]string{".env": realEnv})
	in := filepath.Join(dir, ".env")
	r := runCLI(t, "sanitize", in, "-o", filepath.Join(dir, ".", ".env"))
	if r.code != ExitError || !strings.Contains(r.stderr, "overwrite the input") {
		t.Errorf("got %+v", r)
	}
	if got, _ := os.ReadFile(in); string(got) != realEnv {
		t.Error("input was clobbered")
	}
}
