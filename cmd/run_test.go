package cmd

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// TestMain lets tests run the real CLI in a subprocess, which `envy run`
// needs because it replaces the calling process.
func TestMain(m *testing.M) {
	if os.Getenv("ENVY_TEST_RUN_CLI") == "1" {
		os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

func runCLIProcess(t *testing.T, dir string, env []string, args ...string) (int, string, string) {
	t.Helper()
	c := exec.Command(os.Args[0], args...)
	c.Dir = dir
	c.Env = append(append(os.Environ(), "ENVY_TEST_RUN_CLI=1"), env...)
	var out, errOut strings.Builder
	c.Stdout, c.Stderr = &out, &errOut
	err := c.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return code, out.String(), errOut.String()
}

func TestRunInjectsAndPropagatesExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	dir := writeFiles(t, map[string]string{
		".env":       "GREETING=hello\nSECRET=\"s p a c e s\"\nBASE=http://h\n",
		".env.local": "GREETING=override\nURL=${BASE}/api\n",
	})
	code, out, stderr := runCLIProcess(t, dir, []string{"FROM_SHELL=kept"},
		"run", "-e", ".env", "-e", ".env.local", "--",
		"sh", "-c", `printf '%s|%s|%s|%s\n' "$GREETING" "$SECRET" "$URL" "$FROM_SHELL"; exit 7`)
	if code != 7 {
		t.Errorf("exit = %d, want 7 (stderr %q)", code, stderr)
	}
	if want := "override|s p a c e s|http://h/api|kept\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestRunDefaultsToDotEnvAndFlagsAfterCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	dir := writeFiles(t, map[string]string{".env": "A=1\n"})
	code, out, _ := runCLIProcess(t, dir, nil, "run", "sh", "-c", `echo "$A" "$0"`, "--env")
	if code != 0 || out != "1 --env\n" {
		t.Errorf("code=%d out=%q: flags after the command must go to the command", code, out)
	}
}

func TestRunIsolatedAndNoOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	dir := writeFiles(t, map[string]string{".env": "A=file\nexport PASS_ME\n"})
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	shellEnv := []string{"A=shell", "PASS_ME=yes", "OTHER=leak"}

	_, out, _ := runCLIProcess(t, dir, shellEnv, "run", "--isolated", "--", sh, "-c", `echo "$A|$PASS_ME|$OTHER"`)
	if out != "file|yes|\n" {
		t.Errorf("isolated: %q", out)
	}
	_, out, _ = runCLIProcess(t, dir, shellEnv, "run", "--no-override", "--", "sh", "-c", `echo "$A"`)
	if out != "shell\n" {
		t.Errorf("no-override: %q", out)
	}
}

func TestRunErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix exit codes")
	}
	dir := writeFiles(t, map[string]string{".env": "A=1\n", "bad.env": "A='x\n", "script": "not executable"})
	tests := []struct {
		name   string
		args   []string
		code   int
		stderr string
	}{
		{"not found", []string{"run", "--", "envy-definitely-missing-cmd"}, exitNotFound, "command not found"},
		{"not executable", []string{"run", "--", "./script"}, exitNotExecutable, "permission denied"},
		{"parse error", []string{"run", "-e", "bad.env", "--", "true"}, ExitError, "unterminated"},
		{"missing env file", []string{"run", "-e", "nope.env", "--", "true"}, ExitError, "no such file"},
		{"no command", []string{"run"}, ExitError, "requires at least 1 arg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, stderr := runCLIProcess(t, dir, nil, tt.args...)
			if code != tt.code || !strings.Contains(stderr, tt.stderr) {
				t.Errorf("code=%d stderr=%q, want %d containing %q", code, stderr, tt.code, tt.stderr)
			}
		})
	}
}
