package audit

import (
	"math"
	"strings"
	"testing"

	"github.com/yassataiseer/envy/pkg/envparse"
)

func parse(t *testing.T, path, s string) *envparse.EnvFile {
	t.Helper()
	f, err := envparse.ParseBytes([]byte(s), path)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func findings(t *testing.T, path, s string) []AuditFinding {
	t.Helper()
	return Audit(parse(t, path, s), DefaultOptions())
}

func byRule(fs []AuditFinding, rule string) []AuditFinding {
	var out []AuditFinding
	for _, f := range fs {
		if f.RuleID == rule {
			out = append(out, f)
		}
	}
	return out
}

func TestShannonEntropy(t *testing.T) {
	tests := []struct {
		in   string
		want float64
	}{
		{"", 0},
		{"aaaa", 0},
		{"ab", 1},
		{"abcd", 2},
		{"0123456789abcdef", 4},
		{"éé", 0},
	}
	for _, tt := range tests {
		if got := CalculateShannonEntropy(tt.in); math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("entropy(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestPrefixLeak(t *testing.T) {
	tests := []struct {
		line string
		sev  Severity // "" means no finding
	}{
		{"NEXT_PUBLIC_STRIPE_SECRET=sk_123", SeverityCritical},
		{"VITE_DB_PASSWORD=hunter2", SeverityCritical},
		{"REACT_APP_PRIVATE_KEY=abc", SeverityCritical},
		{"NUXT_PUBLIC_CLIENTSECRET=abc", SeverityCritical},
		{"NEXT_PUBLIC_SUPABASE_SERVICE_ROLE=eyJ", SeverityCritical},
		{"NEXT_PUBLIC_ANALYTICS=sk_test_abcdefgh_ijklmnop_qrstuvwx", SeverityCritical},
		{"GATSBY_API_TOKEN=abc", SeverityWarning},
		{"EXPO_PUBLIC_API_KEY=abc", SeverityWarning},
		{"NEXT_PUBLIC_STRIPE_PUBLISHABLE_KEY=pk_live_abc", ""},
		{"NEXT_PUBLIC_SUPABASE_ANON_KEY=eyJ", ""},
		{"NEXT_PUBLIC_RECAPTCHA_SITE_KEY=abc", ""},
		{"NEXT_PUBLIC_MAPBOX_TOKEN=pk.eyJ1", ""},
		{"NEXT_PUBLIC_POSTHOG_KEY=phc_abc", ""},
		{"NEXT_PUBLIC_APP_URL=http://localhost:3000", ""},
		{"NEXT_PUBLIC_KEYBOARD_LAYOUT=qwerty", ""},
		{"VITE_BYPASS_CACHE=true", ""},
		{"STRIPE_SECRET=sk_123", ""},
		{"NEXT_PUBLIC_=x", ""},
	}
	for _, tt := range tests {
		got := byRule(findings(t, ".env", tt.line), RulePrefixLeak)
		switch {
		case tt.sev == "" && len(got) > 0:
			t.Errorf("%s: unexpected finding %+v", tt.line, got[0])
		case tt.sev != "" && len(got) != 1:
			t.Errorf("%s: got %d findings, want one %s", tt.line, len(got), tt.sev)
		case tt.sev != "" && got[0].Severity != tt.sev:
			t.Errorf("%s: severity %s, want %s", tt.line, got[0].Severity, tt.sev)
		}
	}
}

func TestPrefixLeakMessageMatchesDesignDoc(t *testing.T) {
	fs := byRule(findings(t, ".env", "\n\nNEXT_PUBLIC_STRIPE_SECRET=sk_123"), RulePrefixLeak)
	if len(fs) != 1 {
		t.Fatalf("findings = %+v", fs)
	}
	f := fs[0]
	want := "KEY 'NEXT_PUBLIC_STRIPE_SECRET' contains framework prefix 'NEXT_PUBLIC_' but carries sensitive keyword 'SECRET'. This value will leak to client bundles."
	if f.Message != want || f.LineNumber != 3 || !strings.Contains(f.Remediation, "'STRIPE_SECRET'") {
		t.Errorf("finding = %+v", f)
	}
	if f.Rule != "Framework Prefix Violation" {
		t.Errorf("rule name = %q", f.Rule)
	}
}

func TestEntropy(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"Zm9vYmFyYmF6cXV4cXV1eGNvcmdlZ3JhdWx0Z2FycGx5", true},
		{"q8F!zR2@vL9#mX4$wT7^nB1&kP6*", true},
		{"9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", true},
		{"short", false},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
		{"this is a long sentence with many letters", false},
		{"https://example.com/abcdefghijklmnopqrstuvwxyz0123456789", false},
		{"${OTHER_VARIABLE_REFERENCE_123456789}", false},
		{"0123456789abcdef", false},
	}
	for _, tt := range tests {
		got := len(byRule(findings(t, ".env", "V='"+tt.value+"'"), RuleEntropy)) > 0
		if got != tt.want {
			t.Errorf("%q: flagged=%v want %v (entropy %.2f)", tt.value, got, tt.want, CalculateShannonEntropy(tt.value))
		}
	}
}

func TestEntropyMessageNeverIncludesValue(t *testing.T) {
	secret := "Zm9vYmFyYmF6cXV4cXV1eGNvcmdlZ3JhdWx0Z2FycGx5"
	for _, f := range findings(t, ".env", "JWT_SIGNING_KEY="+secret) {
		if strings.Contains(f.Message, secret) || strings.Contains(f.Remediation, secret) {
			t.Errorf("finding leaks the value: %+v", f)
		}
	}
}

func TestKnownFormats(t *testing.T) {
	tests := map[string]string{
		"sk_" + "live_4eC39HqLyjWDarjtT1zdp7dc":                               "Stripe live secret key",
		"ghp_abcdefghijklmnopqrstuvwxyz0123456789":                            "GitHub personal access token",
		"AKIAIOSFODNN7EXAMPLE":                                                "AWS access key ID",
		"xoxb-123456789012-abcdefghijkl":                                      "Slack bot token",
		"sk-ant-api03-abcdefghijklmnopqrstuvwxyz":                             "Anthropic API key",
		"-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----": "PEM private key",
		"AKIA_not_an_aws_key":                                                 "",
		"sk_test_short":                                                       "",
		"pk_live_4eC39HqLyjWDarjtT1zdp7dc":                                    "",
	}
	for value, want := range tests {
		if got := matchSecretFormat(value); got != want {
			t.Errorf("matchSecretFormat(%q) = %q, want %q", value, got, want)
		}
	}
}

func TestKnownFormatSupersedesEntropy(t *testing.T) {
	fs := findings(t, ".env", "STRIPE_KEY=sk_test_4eC39HqL_yjWDarjt_T1zdp7dc_AbCdEfGh")
	if len(byRule(fs, RuleKnownFormat)) != 1 || len(byRule(fs, RuleEntropy)) != 0 {
		t.Errorf("findings = %+v", fs)
	}
}

func TestTemplateEscalation(t *testing.T) {
	line := "STRIPE_KEY=sk_test_4eC39HqL_yjWDarjt_T1zdp7dc\nDB=postgres://app:s3cr3t@db.prod.internal/app\n"
	for path, want := range map[string]Severity{".env": SeverityWarning, ".env.example": SeverityCritical, "config/example.env": SeverityCritical} {
		for _, f := range findings(t, path, line) {
			if (f.RuleID == RuleKnownFormat || f.RuleID == RuleURLCredentials) && f.Severity != want {
				t.Errorf("%s %s: severity %s, want %s", path, f.RuleID, f.Severity, want)
			}
		}
	}
	if !IsTemplatePath(".env.sample") || IsTemplatePath(".env.production") {
		t.Error("IsTemplatePath misclassified")
	}
}

func TestURLCredentials(t *testing.T) {
	tests := []struct {
		value string
		sev   Severity
	}{
		{"postgres://app:s3cr3t@db.prod.internal:5432/app", SeverityWarning},
		{"postgres://postgres:postgres@localhost:5432/app", SeverityInfo},
		{"redis://:hunter2@cache.example.com:6379", SeverityWarning},
		{"postgres://app:password@db/app", ""},
		{"postgres://app:${DB_PASSWORD}@db/app", ""},
		{"postgres://app@db/app", ""},
		{"https://example.com", ""},
	}
	for _, tt := range tests {
		fs := byRule(findings(t, ".env", "DATABASE_URL=\""+tt.value+"\""), RuleURLCredentials)
		switch {
		case tt.sev == "" && len(fs) > 0:
			t.Errorf("%s: unexpected %+v", tt.value, fs[0])
		case tt.sev != "" && (len(fs) != 1 || fs[0].Severity != tt.sev):
			t.Errorf("%s: got %+v, want %s", tt.value, fs, tt.sev)
		case len(fs) == 1 && strings.Contains(fs[0].Message, "s3cr3t"):
			t.Errorf("message leaks the password: %s", fs[0].Message)
		}
	}
}

func TestQualityRules(t *testing.T) {
	src := "export A=1\nB=hello world  \nA=2\nC=\"quoted value\"\nD=a;b\n"
	fs := findings(t, ".env", src)

	dups := byRule(fs, RuleDuplicateKey)
	if len(dups) != 1 || dups[0].LineNumber != 1 || !strings.Contains(dups[0].Message, "overridden by line 3") {
		t.Errorf("duplicates = %+v", dups)
	}
	ws := byRule(fs, RuleTrailingWhitespace)
	if len(ws) != 1 || ws[0].LineNumber != 2 {
		t.Errorf("trailing whitespace = %+v", ws)
	}
	uq := byRule(fs, RuleUnquotedComplex)
	if len(uq) != 2 || uq[0].Key != "B" || uq[1].Key != "D" {
		t.Errorf("unquoted = %+v", uq)
	}
	ex := byRule(fs, RuleInconsistentExport)
	if len(ex) != 1 || ex[0].Key != "B" {
		t.Errorf("export = %+v", ex)
	}
	if len(byRule(findings(t, ".env", "A=1\nB=2\n"), RuleInconsistentExport)) != 0 {
		t.Error("consistent file flagged")
	}
}

func TestIgnoreDirective(t *testing.T) {
	src := `NEXT_PUBLIC_API_KEY=abc # envy:ignore SEC001
NEXT_PUBLIC_DB_PASSWORD=x # envy:ignore
NEXT_PUBLIC_OTHER_SECRET=y # envy:ignore QLT001, sec002
`
	fs := findings(t, ".env", src)
	if len(fs) != 1 || fs[0].Key != "NEXT_PUBLIC_OTHER_SECRET" {
		t.Errorf("findings = %+v", fs)
	}
}

func TestSortedBySeverityThenLine(t *testing.T) {
	src := "B=hello world\nNEXT_PUBLIC_SECRET=x\nA=1\nA=2\n"
	fs := findings(t, ".env", src)
	for i := 1; i < len(fs); i++ {
		a, b := fs[i-1], fs[i]
		if a.Severity.Rank() < b.Severity.Rank() || (a.Severity == b.Severity && a.LineNumber > b.LineNumber) {
			t.Errorf("not sorted: %v before %v", a, b)
		}
	}
	if fs[0].Severity != SeverityCritical {
		t.Errorf("first finding = %+v", fs[0])
	}
	if s := Summarize(fs); s.Critical != 1 || s.Warning != 1 || s.Info != 1 {
		t.Errorf("summary = %+v", s)
	}
}

func TestIsSensitive(t *testing.T) {
	f := parse(t, ".env", `PORT=3000
NODE_ENV=development
APP_URL=http://localhost:3000
STRIPE_SECRET_KEY=sk_test_abc
GITHUB_TOKEN=x
SESSION_SALT=x
DATABASE_URL=postgres://u:p4ss@db/app
WEIRD=q8F2zR9vL4mX7wT1nB6kP3jH5
API_KEY=
FEATURE_FLAG=true
`)
	want := map[string]bool{
		"PORT": false, "NODE_ENV": false, "APP_URL": false, "STRIPE_SECRET_KEY": true,
		"GITHUB_TOKEN": true, "SESSION_SALT": true, "DATABASE_URL": true, "WEIRD": true,
		"API_KEY": false, "FEATURE_FLAG": false,
	}
	for key, w := range want {
		e, _ := f.Get(key)
		if got := IsSensitive(e); got != w {
			t.Errorf("IsSensitive(%s) = %v, want %v", key, got, w)
		}
	}
}

func TestTestdataNextjs(t *testing.T) {
	f, err := envparse.ParseFile("../envparse/testdata/nextjs.env")
	if err != nil {
		t.Fatal(err)
	}
	fs := Audit(f, DefaultOptions())
	crit := 0
	for _, fd := range fs {
		if fd.Severity == SeverityCritical {
			crit++
			if fd.Key != "NEXT_PUBLIC_STRIPE_SECRET_KEY" {
				t.Errorf("unexpected critical: %+v", fd)
			}
		}
		if fd.Key == "NEXT_PUBLIC_POSTHOG_KEY" || fd.Key == "NEXT_PUBLIC_APP_URL" {
			t.Errorf("false positive: %+v", fd)
		}
	}
	if crit != 1 {
		t.Errorf("criticals = %d, findings:\n%+v", crit, fs)
	}
}
