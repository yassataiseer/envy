package audit

import (
	"net/url"
	"strings"

	"github.com/yassataiseer/envy/pkg/envparse"
)

// nameTokens splits a key such as "STRIPE_SECRET_KEY" into upper-case
// words separated by '_' or '.'.
func nameTokens(key string) []string {
	return strings.FieldsFunc(strings.ToUpper(key), func(r rune) bool { return r == '_' || r == '.' })
}

// Words that mark a key as holding a secret regardless of other words.
var strongSecretWords = map[string]bool{
	"SECRET": true, "SECRETS": true, "PRIVATE": true, "PASSWORD": true, "PASSWD": true,
	"PASS": true, "PWD": true, "CREDENTIAL": true, "CREDENTIALS": true,
}

// Words that usually mark a secret but are also used for publishable
// identifiers, such as Stripe publishable keys or Supabase anon keys.
var weakSecretWords = map[string]bool{
	"KEY": true, "APIKEY": true, "TOKEN": true, "AUTH": true,
}

// Words that indicate a key is intentionally public.
var publicHintWords = map[string]bool{
	"PUBLISHABLE": true, "PUBLIC": true, "ANON": true, "SITE": true, "BROWSER": true,
}

type nameClass struct {
	strong     string // first strong secret word found
	weak       string // first weak secret word found
	publicHint bool
}

func classifyName(tokens []string) nameClass {
	var c nameClass
	for i, t := range tokens {
		switch {
		case strongSecretWords[t], strings.HasSuffix(t, "SECRET"), strings.HasSuffix(t, "PASSWORD"):
			if c.strong == "" {
				c.strong = t
			}
		case t == "SERVICE" && i+1 < len(tokens) && tokens[i+1] == "ROLE":
			if c.strong == "" {
				c.strong = "SERVICE_ROLE"
			}
		case weakSecretWords[t], strings.HasSuffix(t, "TOKEN"), strings.HasSuffix(t, "APIKEY"):
			if c.weak == "" {
				c.weak = t
			}
		case publicHintWords[t]:
			c.publicHint = true
		}
	}
	return c
}

// credentialFormat recognizes a well-known credential by its prefix.
type credentialFormat struct {
	name   string
	prefix string
	minLen int
	// body restricts the characters after the prefix.
	body func(byte) bool
}

func tokenChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.'
}

func upperAlnum(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

var secretFormats = []credentialFormat{
	{"Stripe live secret key", "sk_live_", 24, tokenChar},
	{"Stripe test secret key", "sk_test_", 24, tokenChar},
	{"Stripe restricted key", "rk_live_", 24, tokenChar},
	{"Stripe webhook signing secret", "whsec_", 20, tokenChar},
	{"GitHub personal access token", "ghp_", 36, tokenChar},
	{"GitHub OAuth token", "gho_", 36, tokenChar},
	{"GitHub user-to-server token", "ghu_", 36, tokenChar},
	{"GitHub server-to-server token", "ghs_", 36, tokenChar},
	{"GitHub fine-grained token", "github_pat_", 40, tokenChar},
	{"GitLab personal access token", "glpat-", 24, tokenChar},
	{"Slack bot token", "xoxb-", 24, tokenChar},
	{"Slack user token", "xoxp-", 24, tokenChar},
	{"Slack app token", "xapp-", 24, tokenChar},
	{"OpenAI API key", "sk-proj-", 32, tokenChar},
	{"Anthropic API key", "sk-ant-", 32, tokenChar},
	{"npm access token", "npm_", 36, tokenChar},
	{"SendGrid API key", "SG.", 40, tokenChar},
	{"AWS access key ID", "AKIA", 20, upperAlnum},
	{"AWS temporary access key ID", "ASIA", 20, upperAlnum},
}

// Prefixes of credentials designed to be shipped to browsers.
var publicFormats = []string{"pk_live_", "pk_test_", "pk.", "phc_", "AIza"}

// matchSecretFormat returns the name of the credential format value
// matches, or "".
func matchSecretFormat(value string) string {
	if strings.Contains(value, "-----BEGIN") && strings.Contains(value, "PRIVATE KEY-----") {
		return "PEM private key"
	}
	for _, f := range secretFormats {
		if len(value) < f.minLen || !strings.HasPrefix(value, f.prefix) {
			continue
		}
		ok := true
		for i := len(f.prefix); i < len(value); i++ {
			if !f.body(value[i]) {
				ok = false
				break
			}
		}
		if ok {
			return f.name
		}
	}
	return ""
}

func isPublicFormat(value string) bool {
	for _, p := range publicFormats {
		if strings.HasPrefix(value, p) {
			return true
		}
	}
	return false
}

// urlPassword returns the password embedded in a URL value, if any.
func urlPassword(value string) (password, host string, ok bool) {
	if !strings.Contains(value, "://") {
		return "", "", false
	}
	u, err := url.Parse(value)
	if err != nil || u.User == nil {
		return "", "", false
	}
	p, set := u.User.Password()
	if !set || p == "" {
		return "", "", false
	}
	return p, u.Hostname(), true
}

var placeholderPasswords = map[string]bool{
	"password": true, "pass": true, "changeme": true, "secret": true, "example": true,
	"xxx": true, "xxxx": true, "****": true, "your_password": true, "yourpassword": true,
}

func isPlaceholder(s string) bool {
	l := strings.ToLower(s)
	return placeholderPasswords[l] || strings.HasPrefix(l, "<") || strings.HasPrefix(l, "${") || strings.HasPrefix(l, "{{")
}

func isLocalHost(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0", "host.docker.internal":
		return true
	}
	return strings.HasSuffix(strings.ToLower(host), ".localhost")
}

// highEntropy reports whether value looks like a random secret, using the
// same exclusions as SEC002.
func highEntropy(value string, threshold, hexThreshold float64) (float64, bool) {
	if len(value) < 20 || strings.ContainsAny(value, " \t\n") || strings.Contains(value, "://") || strings.Contains(value, "${") {
		return 0, false
	}
	h := CalculateShannonEntropy(value)
	if isHex(value) {
		return h, len(value) >= 32 && h >= hexThreshold
	}
	return h, h >= threshold
}

// IsSensitive reports whether an entry probably holds a secret, judging by
// its key name, credential format, entropy, or embedded URL password. It
// is deliberately broader than the audit rules and is meant for deciding
// what to strip from templates.
func IsSensitive(e *envparse.EnvEntry) bool {
	if !e.IsAssignment() || e.Value == "" {
		return false
	}
	c := classifyName(nameTokens(e.Key))
	if c.strong != "" || c.weak != "" {
		return true
	}
	for _, t := range nameTokens(e.Key) {
		switch t {
		case "SALT", "SIGNATURE", "CERT", "DSN", "SESSION", "COOKIE", "JWT", "HMAC", "PRIVKEY":
			return true
		}
	}
	if matchSecretFormat(e.Value) != "" {
		return true
	}
	if _, _, ok := urlPassword(e.Value); ok {
		return true
	}
	d := DefaultOptions()
	_, high := highEntropy(e.Value, d.EntropyThreshold-0.5, d.HexEntropyThreshold)
	return high
}
