package audit

import (
	"fmt"
	"strings"

	"github.com/yassataiseer/envy/pkg/envparse"
)

const (
	RulePrefixLeak     = "SEC001"
	RuleEntropy        = "SEC002"
	RuleKnownFormat    = "SEC003"
	RuleURLCredentials = "SEC004"
)

// frameworkPrefixes are key prefixes that frameworks inline into client
// bundles. Longer prefixes come first so the most specific one matches.
var frameworkPrefixes = []struct{ prefix, framework string }{
	{"NEXT_PUBLIC_", "Next.js"},
	{"NUXT_PUBLIC_", "Nuxt"},
	{"EXPO_PUBLIC_", "Expo"},
	{"REACT_APP_", "Create React App"},
	{"NUXT_ENV_", "Nuxt"},
	{"VUE_APP_", "Vue CLI"},
	{"GATSBY_", "Gatsby"},
	{"VITE_", "Vite"},
	{"PUBLIC_", "SvelteKit/Astro"},
}

func frameworkPrefix(key string) (prefix, framework string) {
	upper := strings.ToUpper(key)
	for _, p := range frameworkPrefixes {
		if strings.HasPrefix(upper, p.prefix) && len(upper) > len(p.prefix) {
			return key[:len(p.prefix)], p.framework
		}
	}
	return "", ""
}

func assignments(f *envparse.EnvFile) []*envparse.EnvEntry {
	out := make([]*envparse.EnvEntry, 0, len(f.Entries))
	for i := range f.Entries {
		if e := &f.Entries[i]; e.IsAssignment() && !e.IsBareExport {
			out = append(out, e)
		}
	}
	return out
}

// prefixLeakRule (SEC001) flags secrets behind framework prefixes that
// expose variables to browser bundles.
type prefixLeakRule struct{}

func (prefixLeakRule) ID() string { return RulePrefixLeak }

func (prefixLeakRule) Evaluate(f *envparse.EnvFile) []AuditFinding {
	var out []AuditFinding
	for _, e := range assignments(f) {
		prefix, framework := frameworkPrefix(e.Key)
		if prefix == "" {
			continue
		}
		rest := e.Key[len(prefix):]
		c := classifyName(nameTokens(rest))
		format := matchSecretFormat(e.Value)
		remediation := fmt.Sprintf("Rename the key to '%s' and read it only in server-side code. If it has been deployed, rotate it: anything in a client bundle is public.", rest)

		fd := AuditFinding{RuleID: RulePrefixLeak, Rule: "Framework Prefix Violation", Key: e.Key, LineNumber: e.LineNumber, Remediation: remediation}
		switch {
		case format != "":
			fd.Severity = SeverityCritical
			fd.Message = fmt.Sprintf("KEY '%s' has the %s client prefix '%s' and its value looks like a %s. This value will leak to client bundles.", e.Key, framework, prefix, format)
		case c.strong != "":
			fd.Severity = SeverityCritical
			fd.Message = fmt.Sprintf("KEY '%s' contains framework prefix '%s' but carries sensitive keyword '%s'. This value will leak to client bundles.", e.Key, prefix, c.strong)
		case c.weak != "" && !c.publicHint && !isPublicFormat(e.Value):
			fd.Severity = SeverityWarning
			fd.Message = fmt.Sprintf("KEY '%s' contains framework prefix '%s' and keyword '%s'; it may be a secret exposed to client bundles.", e.Key, prefix, c.weak)
			fd.Remediation = remediation + " If this is a publishable key, add '# envy:ignore SEC001' to the line."
		default:
			continue
		}
		out = append(out, fd)
	}
	return out
}

// templateEscalation raises findings in committed template files, where
// any real secret is a leak.
func templateEscalation(f *envparse.EnvFile, base Severity) (Severity, string) {
	if IsTemplatePath(f.Path) {
		return SeverityCritical, " Template files are normally committed to version control, so this secret is likely already exposed."
	}
	return base, ""
}

// entropyRule (SEC002) flags values with high Shannon entropy, which
// usually indicates generated secrets.
type entropyRule struct {
	threshold, hexThreshold float64
}

func (entropyRule) ID() string { return RuleEntropy }

func (r entropyRule) Evaluate(f *envparse.EnvFile) []AuditFinding {
	var out []AuditFinding
	for _, e := range assignments(f) {
		h, high := highEntropy(e.Value, r.threshold, r.hexThreshold)
		if !high {
			continue
		}
		threshold := r.threshold
		if isHex(e.Value) {
			threshold = r.hexThreshold
		}
		sev, extra := templateEscalation(f, SeverityWarning)
		out = append(out, AuditFinding{
			RuleID:      RuleEntropy,
			Rule:        "High Entropy Value Detected",
			Severity:    sev,
			Key:         e.Key,
			LineNumber:  e.LineNumber,
			Message:     fmt.Sprintf("KEY '%s' has Shannon entropy score of %.2f (> %.1f threshold).%s", e.Key, h, threshold, extra),
			Remediation: "Ensure this file is listed in .gitignore and never committed to version control.",
		})
	}
	return out
}

// knownFormatRule (SEC003) recognizes well-known credential formats.
type knownFormatRule struct{}

func (knownFormatRule) ID() string { return RuleKnownFormat }

func (knownFormatRule) Evaluate(f *envparse.EnvFile) []AuditFinding {
	var out []AuditFinding
	for _, e := range assignments(f) {
		name := matchSecretFormat(e.Value)
		if name == "" {
			continue
		}
		sev, extra := templateEscalation(f, SeverityWarning)
		out = append(out, AuditFinding{
			RuleID:      RuleKnownFormat,
			Rule:        "Known Credential Format",
			Severity:    sev,
			Key:         e.Key,
			LineNumber:  e.LineNumber,
			Message:     fmt.Sprintf("KEY '%s' holds what looks like a %s.%s", e.Key, name, extra),
			Remediation: "Keep this file out of version control. If the credential was ever committed or shared, revoke and rotate it.",
		})
	}
	return out
}

// urlCredentialsRule (SEC004) flags passwords embedded in connection URLs.
type urlCredentialsRule struct{}

func (urlCredentialsRule) ID() string { return RuleURLCredentials }

func (urlCredentialsRule) Evaluate(f *envparse.EnvFile) []AuditFinding {
	var out []AuditFinding
	for _, e := range assignments(f) {
		password, host, ok := urlPassword(e.Value)
		if !ok || isPlaceholder(password) {
			continue
		}
		sev, extra := SeverityWarning, ""
		if isLocalHost(host) {
			sev = SeverityInfo
		} else {
			sev, extra = templateEscalation(f, sev)
		}
		out = append(out, AuditFinding{
			RuleID:      RuleURLCredentials,
			Rule:        "Credentials Embedded in URL",
			Severity:    sev,
			Key:         e.Key,
			LineNumber:  e.LineNumber,
			Message:     fmt.Sprintf("KEY '%s' embeds a password in its URL for host '%s'.%s", e.Key, host, extra),
			Remediation: "Keep this file out of version control, or move the password to its own variable that your deployment platform injects.",
		})
	}
	return out
}
