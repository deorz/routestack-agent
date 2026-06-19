// Package certs manages public TLS certificates through Certbot.
package certs

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	ActionIssue  = "issue"
	ActionRenew  = "renew"
	ActionRevoke = "revoke"

	MethodStandalone = "standalone"
	MethodWebroot    = "webroot"

	defaultCertbotPath = "certbot"
	letsencryptLiveDir = "/etc/letsencrypt/live"
)

// Request describes a certificate management operation from the control plane.
type Request struct {
	Domain       string   `json:"domain"`
	Domains      []string `json:"domains,omitempty"`
	Action       string   `json:"action"`
	Method       string   `json:"method,omitempty"`
	Email        string   `json:"email,omitempty"`
	WebrootPath  string   `json:"webroot_path,omitempty"`
	Staging      bool     `json:"staging,omitempty"`
	ForceRenewal bool     `json:"force_renewal,omitempty"`
	RevokeReason string   `json:"revoke_reason,omitempty"`
}

// Report is returned to the control plane after Certbot exits successfully.
type Report struct {
	Action         string   `json:"action"`
	Domain         string   `json:"domain"`
	Domains        []string `json:"domains,omitempty"`
	Method         string   `json:"method,omitempty"`
	Staging        bool     `json:"staging,omitempty"`
	CertificateDir string   `json:"certificate_dir,omitempty"`
	FullchainPath  string   `json:"fullchain_path,omitempty"`
	PrivateKeyPath string   `json:"private_key_path,omitempty"`
	Output         string   `json:"output,omitempty"`
}

type commandRunner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Manager runs Certbot with validated, non-interactive arguments.
type Manager struct {
	certbotPath string
	runner      commandRunner
}

// NewManager creates a certificate manager using certbot from PATH.
func NewManager() *Manager {
	return &Manager{certbotPath: defaultCertbotPath, runner: execRunner{}}
}

// NewManagerWithBinary creates a manager using an explicit certbot binary path.
func NewManagerWithBinary(path string) *Manager {
	if path == "" {
		path = defaultCertbotPath
	}
	return &Manager{certbotPath: path, runner: execRunner{}}
}

// Manage validates and executes a certificate operation.
func (m *Manager) Manage(ctx context.Context, req Request) (*Report, error) {
	args, report, err := buildCertbotCommand(req)
	if err != nil {
		return nil, err
	}

	out, err := m.runner.Run(ctx, m.certbotPath, args...)
	report.Output = out
	if err != nil {
		if out != "" {
			return report, fmt.Errorf("certbot %s failed: %w: %s", req.Action, err, out)
		}
		return report, fmt.Errorf("certbot %s failed: %w", req.Action, err)
	}
	return report, nil
}

func buildCertbotCommand(req Request) ([]string, *Report, error) {
	domain, err := normalizeDomain(req.Domain)
	if err != nil {
		return nil, nil, fmt.Errorf("domain: %w", err)
	}
	domains, err := normalizeDomains(domain, req.Domains)
	if err != nil {
		return nil, nil, err
	}

	report := &Report{
		Action:         req.Action,
		Domain:         domain,
		Domains:        domains,
		Staging:        req.Staging,
		CertificateDir: filepath.Join(letsencryptLiveDir, domain),
		FullchainPath:  filepath.Join(letsencryptLiveDir, domain, "fullchain.pem"),
		PrivateKeyPath: filepath.Join(letsencryptLiveDir, domain, "privkey.pem"),
	}

	switch req.Action {
	case ActionIssue:
		args, method, err := issueArgs(req, domain, domains)
		if err != nil {
			return nil, nil, err
		}
		report.Method = method
		return args, report, nil
	case ActionRenew:
		args, err := renewArgs(req, domain)
		if err != nil {
			return nil, nil, err
		}
		return args, report, nil
	case ActionRevoke:
		args, err := revokeArgs(req, domain)
		if err != nil {
			return nil, nil, err
		}
		return args, report, nil
	default:
		return nil, nil, fmt.Errorf("unsupported certificate action %q", req.Action)
	}
}

func issueArgs(req Request, domain string, domains []string) (args []string, method string, err error) {
	if strings.TrimSpace(req.Email) == "" {
		return nil, "", fmt.Errorf("email is required for certificate issuance")
	}
	args = []string{"certonly", "--non-interactive", "--agree-tos", "--cert-name", domain, "--email", strings.TrimSpace(req.Email)}
	methodArgs, method, err := challengeArgs(req.Method, req.WebrootPath)
	if err != nil {
		return nil, "", err
	}
	args = append(args, methodArgs...)
	if req.Staging {
		args = append(args, "--test-cert")
	}
	if req.ForceRenewal {
		args = append(args, "--force-renewal")
	}
	for _, d := range domains {
		args = append(args, "-d", d)
	}
	return args, method, nil
}

func renewArgs(req Request, domain string) ([]string, error) {
	args := []string{"renew", "--non-interactive", "--cert-name", domain}
	if req.WebrootPath != "" {
		if !filepath.IsAbs(req.WebrootPath) {
			return nil, fmt.Errorf("webroot_path must be absolute")
		}
		args = append(args, "--webroot-path", req.WebrootPath)
	}
	if req.Staging {
		args = append(args, "--test-cert")
	}
	if req.ForceRenewal {
		args = append(args, "--force-renewal")
	}
	return args, nil
}

func revokeArgs(req Request, domain string) ([]string, error) {
	args := []string{"revoke", "--non-interactive", "--cert-name", domain, "--delete-after-revoke"}
	if req.RevokeReason != "" {
		if !validRevokeReason(req.RevokeReason) {
			return nil, fmt.Errorf("unsupported revoke_reason %q", req.RevokeReason)
		}
		args = append(args, "--reason", req.RevokeReason)
	}
	return args, nil
}

func challengeArgs(method, webrootPath string) (args []string, normalizedMethod string, err error) {
	if method == "" {
		method = MethodStandalone
	}
	switch method {
	case MethodStandalone:
		if webrootPath != "" {
			return nil, "", fmt.Errorf("webroot_path requires method %q", MethodWebroot)
		}
		return []string{"--standalone"}, method, nil
	case MethodWebroot:
		if webrootPath == "" {
			return nil, "", fmt.Errorf("webroot_path is required for webroot challenge")
		}
		if !filepath.IsAbs(webrootPath) {
			return nil, "", fmt.Errorf("webroot_path must be absolute")
		}
		return []string{"--webroot", "--webroot-path", webrootPath}, method, nil
	default:
		return nil, "", fmt.Errorf("unsupported certificate method %q", method)
	}
}

func normalizeDomains(primary string, extra []string) ([]string, error) {
	domains := []string{primary}
	for _, raw := range extra {
		domain, err := normalizeDomain(raw)
		if err != nil {
			return nil, fmt.Errorf("domains: %w", err)
		}
		seen := false
		for _, existing := range domains {
			if existing == domain {
				seen = true
				break
			}
		}
		if !seen {
			domains = append(domains, domain)
		}
	}
	return domains, nil
}

func normalizeDomain(raw string) (string, error) {
	domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
	if domain == "" {
		return "", fmt.Errorf("is required")
	}
	if len(domain) > 253 {
		return "", fmt.Errorf("is too long")
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return "", fmt.Errorf("must be a fully qualified domain name")
	}
	for _, label := range labels {
		if label == "" {
			return "", fmt.Errorf("contains an empty label")
		}
		if len(label) > 63 {
			return "", fmt.Errorf("label %q is too long", label)
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return "", fmt.Errorf("label %q starts or ends with hyphen", label)
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
				continue
			}
			return "", fmt.Errorf("label %q contains invalid character %q", label, c)
		}
	}
	return domain, nil
}

func validRevokeReason(reason string) bool {
	switch reason {
	case "keycompromise", "affiliationchanged", "superseded", "cessationofoperation":
		return true
	default:
		return false
	}
}
