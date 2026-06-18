package api

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// EnrollRequest is sent to the controller to enroll a new node.
type EnrollRequest struct {
	EnrollmentToken string `json:"enrollment_token"`
	CSR             string `json:"csr"`
	NodeID          string `json:"node_id,omitempty"`
}

// EnrollResponse is the controller's reply with the signed certificate.
type EnrollResponse struct {
	NodeID      string `json:"node_id"`
	Certificate string `json:"certificate"`
	CACert      string `json:"ca_cert"`
}

// GenerateKeyAndCSR creates an Ed25519 keypair and a PEM-encoded CSR.
// The CSR subject CN is set to "routestack-node-<nodeID>" and includes
// nodeID as a DNS SAN. If nodeID is empty, a random name is used.
func GenerateKeyAndCSR(nodeID string) (crypto.PrivateKey, []byte, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate ed25519 key: %w", err)
	}

	if nodeID == "" {
		nodeID = "unknown"
	}

	subject := pkix.Name{
		CommonName: "routestack-node-" + nodeID,
	}

	template := &x509.CertificateRequest{
		Subject:            subject,
		DNSNames:           []string{nodeID},
		SignatureAlgorithm: x509.PureEd25519,
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("create CSR: %w", err)
	}

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})

	return priv, csrPEM, nil
}

// Enroll performs a full enrollment flow: generates keys, creates a CSR,
// sends it to the controller, and saves the resulting certificate and key
// to disk. It returns the enrollment response or an error.
//
// At enrollment time the agent has no client certificate, so this function
// creates a temporary TLS client that only verifies the server against the CA.
func Enroll(ctx context.Context, controllerURL, token, caCertPath string) (*EnrollResponse, error) {
	// Generate keypair + CSR.
	priv, csrPEM, err := GenerateKeyAndCSR("")
	if err != nil {
		return nil, err
	}

	// Create a CA-only TLS client for the enrollment request.
	client, err := NewClientWithCA(controllerURL, caCertPath, 10*time.Second, 60*time.Second)
	if err != nil {
		return nil, fmt.Errorf("create enrollment client: %w", err)
	}

	req := EnrollRequest{
		EnrollmentToken: token,
		CSR:             string(csrPEM),
	}
	resp, err := client.Enroll(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("enroll request: %w", err)
	}

	// Save the private key.
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: mustMarshalPKCS8(priv),
	})

	keyPath := "/etc/routestack/agent/key.pem"
	if err := writeFileAtomic(keyPath, keyPEM, 0600); err != nil {
		return nil, fmt.Errorf("save key: %w", err)
	}

	// Save the signed certificate.
	certPath := "/etc/routestack/agent/cert.pem"
	if err := writeFileAtomic(certPath, []byte(resp.Certificate), 0600); err != nil {
		return nil, fmt.Errorf("save cert: %w", err)
	}

	return resp, nil
}

// mustMarshalPKCS8 converts a private key to PKCS#8 DER.
// Panics on error — only used for Ed25519 keys which never fail.
func mustMarshalPKCS8(key crypto.PrivateKey) []byte {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		panic(fmt.Sprintf("marshal pkcs8: %v", err))
	}
	return der
}

// writeFileAtomic writes data to a temp file in the same directory,
// sets permissions, then atomically renames over the target path.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".rtstack-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	return os.Rename(tmpPath, path)
}
