// Package redact removes secrets from strings before they leave the agent.
package redact

import "regexp"

const marker = "[REDACTED]"

var (
	wireGuardKeyPattern = regexp.MustCompile(`\b((?:PrivateKey|PresharedKey)\s*=\s*)[A-Za-z0-9+/=_-]+`)
	passwordPattern     = regexp.MustCompile(`(?i)\b(password=)[^\s,;&]+`)
	jsonSecretPattern   = regexp.MustCompile(`(?i)("(?:privateKey|preshared_key|secret|password|uuid)"\s*:\s*)("[^"\\]*(?:\\.[^"\\]*)*"|[^,\s}\]]+)`)
	uuidPattern         = regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
)

// RedactSecrets removes private keys, PSKs, passwords, UUIDs, and known JSON
// secret fields from text intended for logs or control-plane responses.
func RedactSecrets(s string) string {
	if s == "" {
		return s
	}
	if wireGuardKeyPattern.MatchString(s) {
		s = wireGuardKeyPattern.ReplaceAllString(s, `${1}`+marker)
	}
	if passwordPattern.MatchString(s) {
		s = passwordPattern.ReplaceAllString(s, `${1}`+marker)
	}
	if jsonSecretPattern.MatchString(s) {
		s = jsonSecretPattern.ReplaceAllString(s, `${1}"`+marker+`"`)
	}
	if uuidPattern.MatchString(s) {
		s = uuidPattern.ReplaceAllString(s, marker)
	}
	return s
}

// SafeError returns err's message with known secret material redacted for API
// responses. Nil errors produce an empty message.
func SafeError(err error) string {
	if err == nil {
		return ""
	}
	return RedactSecrets(err.Error())
}
