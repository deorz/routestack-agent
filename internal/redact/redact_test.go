package redact

import (
	"errors"
	"testing"
)

func TestRedactWireGuardKeys(t *testing.T) {
	input := "PrivateKey = abcdefghijklmnopqrstuvwxyz0123456789+/=\nPresharedKey = zyxwvutsrqponmlkjihgfedcba9876543210+/="
	want := "PrivateKey = [REDACTED]\nPresharedKey = [REDACTED]"
	if got := RedactSecrets(input); got != want {
		t.Fatalf("RedactSecrets() = %q, want %q", got, want)
	}
}

func TestRedactPasswordTokens(t *testing.T) {
	input := "dial failed: user=agent password=correct-horse-battery-staple host=example"
	want := "dial failed: user=agent password=[REDACTED] host=example"
	if got := RedactSecrets(input); got != want {
		t.Fatalf("RedactSecrets() = %q, want %q", got, want)
	}
}

func TestRedactJSONSecretFields(t *testing.T) {
	input := `{"privateKey":"abc","preshared_key":"def","secret":"ghi","password":"jkl","uuid":"node-1","public":"keep"}`
	want := `{"privateKey":"[REDACTED]","preshared_key":"[REDACTED]","secret":"[REDACTED]","password":"[REDACTED]","uuid":"[REDACTED]","public":"keep"}`
	if got := RedactSecrets(input); got != want {
		t.Fatalf("RedactSecrets() = %q, want %q", got, want)
	}
}

func TestRedactCanonicalUUIDs(t *testing.T) {
	input := "operation 123e4567-e89b-12d3-a456-426614174000 failed"
	want := "operation [REDACTED] failed"
	if got := RedactSecrets(input); got != want {
		t.Fatalf("RedactSecrets() = %q, want %q", got, want)
	}
}

func TestSafeError(t *testing.T) {
	if got := SafeError(nil); got != "" {
		t.Fatalf("SafeError(nil) = %q, want empty", got)
	}
	err := errors.New("api rejected password=swordfish")
	if got := SafeError(err); got != "api rejected password=[REDACTED]" {
		t.Fatalf("SafeError() = %q", got)
	}
}
