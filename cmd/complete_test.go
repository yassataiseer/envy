package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func completions(t *testing.T, args ...string) []string {
	t.Helper()
	r := runCLI(t, append([]string{"__complete"}, args...)...)
	if r.code != 0 {
		t.Fatalf("__complete failed: %+v", r)
	}
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(r.stdout), "\n") {
		if l != "" && !strings.HasPrefix(l, ":") {
			out = append(out, strings.SplitN(l, "\t", 2)[0])
		}
	}
	return out
}

func TestEnvFileCompletion(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		".env": "", ".env.example": "", "prod.env": "", "README.md": "", ".gitignore": "",
	})
	if err := os.Mkdir(filepath.Join(dir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", ".env.test"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	want := ".env .env.example prod.env"
	for _, args := range [][]string{
		{"diff", ""},
		{"diff", ".env", ""},
		{"audit", ""},
		{"sanitize", ""},
		{"run", "--env", ""},
		{"sanitize", ".env", "-o", ""},
	} {
		if got := strings.Join(completions(t, args...), " "); got != want {
			t.Errorf("%v: got %q, want %q", args, got, want)
		}
	}
	if got := completions(t, "diff", ".env", ".env.example", ""); len(got) != 0 {
		t.Errorf("third diff arg should not complete, got %v", got)
	}
	if got := completions(t, "diff", "config/"); strings.Join(got, " ") != "config/.env.test" {
		t.Errorf("subdirectory completion = %v", got)
	}
	if got := completions(t, "diff", ".env."); strings.Join(got, " ") != ".env.example" {
		t.Errorf("prefix completion = %v", got)
	}
}
