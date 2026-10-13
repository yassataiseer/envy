package sanitize

import (
	"strings"
	"testing"

	"github.com/yassataiseer/envy/pkg/audit"
	"github.com/yassataiseer/envy/pkg/envparse"
)

const input = `# App config
PORT=3000   # http port
export NODE_ENV=development
STRIPE_SECRET_KEY=sk_test_4eC39HqL_yjWDarjt_T1zdp7dc
DATABASE_URL="postgres://app:s3cr3t@db.prod.internal/app"
PUBLIC_URL=http://localhost:3000
PRIVATE_KEY="-----BEGIN PRIVATE KEY-----
abc
-----END PRIVATE KEY-----"
EMPTY=
export INHERITED
# OLD_TOKEN=ghp_abcdefghijklmnopqrstuvwxyz0123456789 # rotated
# Copy this file to .env and fill it in.
API_TOKEN=first
API_TOKEN=second
`

func run(t *testing.T, opts Options) (string, Result) {
	t.Helper()
	f, err := envparse.ParseBytes([]byte(input), ".env")
	if err != nil {
		t.Fatal(err)
	}
	res := Sanitize(f, opts)
	return string(f.Format()), res
}

func TestSanitizeDefaultStripsEverything(t *testing.T) {
	got, res := run(t, Options{})
	want := `# App config
PORT= # http port
export NODE_ENV=
STRIPE_SECRET_KEY=
DATABASE_URL=
PUBLIC_URL=
PRIVATE_KEY=
EMPTY=
export INHERITED
# OLD_TOKEN= # rotated
# Copy this file to .env and fill it in.
API_TOKEN=
API_TOKEN=
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if res != (Result{Stripped: 8, Comments: 1}) {
		t.Errorf("result = %+v", res)
	}
}

func TestSanitizeKeepSafe(t *testing.T) {
	got, res := run(t, Options{KeepSafe: true})
	for _, keep := range []string{"PORT=3000   # http port", "export NODE_ENV=development", "PUBLIC_URL=http://localhost:3000"} {
		if !strings.Contains(got, keep) {
			t.Errorf("safe line %q not kept:\n%s", keep, got)
		}
	}
	for _, gone := range []string{"sk_live", "s3cr3t", "BEGIN PRIVATE", "ghp_", "first", "second"} {
		if strings.Contains(got, gone) {
			t.Errorf("sensitive value %q kept:\n%s", gone, got)
		}
	}
	if res.Kept != 3 || res.Stripped != 5 {
		t.Errorf("result = %+v", res)
	}
}

func TestSanitizeMask(t *testing.T) {
	got, _ := run(t, Options{Mask: true})
	for _, want := range []string{
		`PORT="<number>" # http port`,
		`export NODE_ENV="<value>"`,
		`STRIPE_SECRET_KEY="<secret>"`,
		`DATABASE_URL="<secret>"`,
		`PUBLIC_URL="<url>"`,
		`# OLD_TOKEN="<secret>" # rotated`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestSanitizedOutputPassesAudit(t *testing.T) {
	for _, opts := range []Options{{}, {KeepSafe: true}, {Mask: true}} {
		got, _ := run(t, opts)
		f, err := envparse.ParseBytes([]byte(got), ".env.example")
		if err != nil {
			t.Fatalf("%+v: output does not parse: %v", opts, err)
		}
		for _, fd := range audit.Audit(f, audit.DefaultOptions()) {
			if strings.HasPrefix(fd.RuleID, "SEC") {
				t.Errorf("%+v: sanitized template still has %s %s on line %d", opts, fd.Severity, fd.RuleID, fd.LineNumber)
			}
		}
	}
}
