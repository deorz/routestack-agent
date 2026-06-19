package certs

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestManageIssueStandalone(t *testing.T) {
	certbot, argsFile := fakeCertbot(t, 0)
	mgr := NewManagerWithBinary(certbot)

	report, err := mgr.Manage(context.Background(), Request{
		Action:  ActionIssue,
		Domain:  "Example.COM.",
		Domains: []string{"www.example.com", "example.com"},
		Email:   "ops@example.com",
		Staging: true,
	})
	if err != nil {
		t.Fatalf("Manage: %v", err)
	}

	wantArgs := []string{
		"certonly", "--non-interactive", "--agree-tos", "--cert-name", "example.com",
		"--email", "ops@example.com", "--standalone", "--test-cert",
		"-d", "example.com", "-d", "www.example.com",
	}
	if got := recordedArgs(t, argsFile); !reflect.DeepEqual(got, wantArgs) {
		t.Fatalf("args:\n got: %#v\nwant: %#v", got, wantArgs)
	}
	if report.Domain != "example.com" || report.Method != MethodStandalone {
		t.Fatalf("report domain/method: %#v", report)
	}
	if report.FullchainPath != "/etc/letsencrypt/live/example.com/fullchain.pem" {
		t.Fatalf("fullchain path: %q", report.FullchainPath)
	}
}

func TestManageIssueWebroot(t *testing.T) {
	certbot, argsFile := fakeCertbot(t, 0)
	mgr := NewManagerWithBinary(certbot)

	_, err := mgr.Manage(context.Background(), Request{
		Action:       ActionIssue,
		Domain:       "api.example.com",
		Email:        "ops@example.com",
		Method:       MethodWebroot,
		WebrootPath:  "/var/www/acme",
		ForceRenewal: true,
	})
	if err != nil {
		t.Fatalf("Manage: %v", err)
	}

	wantArgs := []string{
		"certonly", "--non-interactive", "--agree-tos", "--cert-name", "api.example.com",
		"--email", "ops@example.com", "--webroot", "--webroot-path", "/var/www/acme",
		"--force-renewal", "-d", "api.example.com",
	}
	if got := recordedArgs(t, argsFile); !reflect.DeepEqual(got, wantArgs) {
		t.Fatalf("args:\n got: %#v\nwant: %#v", got, wantArgs)
	}
}

func TestManageRenew(t *testing.T) {
	certbot, argsFile := fakeCertbot(t, 0)
	mgr := NewManagerWithBinary(certbot)

	_, err := mgr.Manage(context.Background(), Request{
		Action:       ActionRenew,
		Domain:       "example.com",
		WebrootPath:  "/srv/www",
		ForceRenewal: true,
	})
	if err != nil {
		t.Fatalf("Manage: %v", err)
	}

	wantArgs := []string{"renew", "--non-interactive", "--cert-name", "example.com", "--webroot-path", "/srv/www", "--force-renewal"}
	if got := recordedArgs(t, argsFile); !reflect.DeepEqual(got, wantArgs) {
		t.Fatalf("args:\n got: %#v\nwant: %#v", got, wantArgs)
	}
}

func TestManageRevoke(t *testing.T) {
	certbot, argsFile := fakeCertbot(t, 0)
	mgr := NewManagerWithBinary(certbot)

	_, err := mgr.Manage(context.Background(), Request{
		Action:       ActionRevoke,
		Domain:       "example.com",
		RevokeReason: "superseded",
	})
	if err != nil {
		t.Fatalf("Manage: %v", err)
	}

	wantArgs := []string{"revoke", "--non-interactive", "--cert-name", "example.com", "--delete-after-revoke", "--reason", "superseded"}
	if got := recordedArgs(t, argsFile); !reflect.DeepEqual(got, wantArgs) {
		t.Fatalf("args:\n got: %#v\nwant: %#v", got, wantArgs)
	}
}

func TestManageRejectsInvalidPayload(t *testing.T) {
	certbot, _ := fakeCertbot(t, 0)
	mgr := NewManagerWithBinary(certbot)

	cases := []Request{
		{Action: ActionIssue, Domain: "localhost", Email: "ops@example.com"},
		{Action: ActionIssue, Domain: "example.com"},
		{Action: ActionIssue, Domain: "example.com", Email: "ops@example.com", Method: MethodWebroot},
		{Action: ActionIssue, Domain: "example.com", Email: "ops@example.com", Method: MethodWebroot, WebrootPath: "relative"},
		{Action: ActionRevoke, Domain: "example.com", RevokeReason: "bad"},
		{Action: "delete", Domain: "example.com"},
	}

	for _, tc := range cases {
		if _, err := mgr.Manage(context.Background(), tc); err == nil {
			t.Fatalf("Manage(%+v) succeeded, want error", tc)
		}
	}
}

func TestManageReturnsCertbotFailure(t *testing.T) {
	certbot, _ := fakeCertbot(t, 3)
	mgr := NewManagerWithBinary(certbot)

	report, err := mgr.Manage(context.Background(), Request{
		Action: ActionRenew,
		Domain: "example.com",
	})
	if err == nil {
		t.Fatal("Manage succeeded, want error")
	}
	if report == nil || report.Output != "certbot-output" {
		t.Fatalf("report: %#v", report)
	}
	if !strings.Contains(err.Error(), "certbot renew failed") || !strings.Contains(err.Error(), "certbot-output") {
		t.Fatalf("error: %v", err)
	}
}

func fakeCertbot(t *testing.T, exitCode int) (certbotPath, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args")
	certbotPath = filepath.Join(dir, "certbot")
	script := `#!/bin/sh
: > "$CERTBOT_ARGS_FILE"
for arg in "$@"; do
  printf '%s\n' "$arg" >> "$CERTBOT_ARGS_FILE"
done
printf 'certbot-output'
exit $CERTBOT_EXIT_CODE
`
	if err := os.WriteFile(certbotPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write certbot script: %v", err)
	}
	t.Setenv("CERTBOT_ARGS_FILE", argsFile)
	t.Setenv("CERTBOT_EXIT_CODE", string(rune('0'+exitCode)))
	return certbotPath, argsFile
}

func recordedArgs(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	text := strings.TrimSuffix(string(data), "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}
