package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStateNonexistentReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")

	st, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState on nonexistent file: %v", err)
	}
	if st == nil {
		t.Fatal("LoadState returned nil")
	}
	if st.IsEnrolled() {
		t.Error("empty state should not report as enrolled")
	}
	if st.NodeID != "" {
		t.Errorf("empty state NodeID should be empty, got %q", st.NodeID)
	}
}

func TestSaveAndLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Write state.
	st, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	st.NodeID = "node-abc123"
	st.CertPath = "/etc/routestack/agent/cert.pem"
	st.KeyPath = "/etc/routestack/agent/key.pem"
	st.LastKnownRevisions["xray"] = 5
	st.LastKnownRevisions["haproxy"] = 2

	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Read back and verify.
	st2, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState after save: %v", err)
	}

	if st2.NodeID != "node-abc123" {
		t.Errorf("NodeID: got %q, want %q", st2.NodeID, "node-abc123")
	}
	if st2.CertPath != "/etc/routestack/agent/cert.pem" {
		t.Errorf("CertPath: got %q", st2.CertPath)
	}
	if st2.KeyPath != "/etc/routestack/agent/key.pem" {
		t.Errorf("KeyPath: got %q", st2.KeyPath)
	}
	if v := st2.LastKnownRevisions["xray"]; v != 5 {
		t.Errorf("LastKnownRevisions[xray]: got %d, want 5", v)
	}
	if v := st2.LastKnownRevisions["haproxy"]; v != 2 {
		t.Errorf("LastKnownRevisions[haproxy]: got %d, want 2", v)
	}
}

func TestIsEnrolled(t *testing.T) {
	dir := t.TempDir()

	// Create fake cert and key files.
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	os.WriteFile(certPath, []byte("fake-cert"), 0600)
	os.WriteFile(keyPath, []byte("fake-key"), 0600)

	st := &AgentState{
		NodeID:   "node-xyz",
		CertPath: certPath,
		KeyPath:  keyPath,
	}

	if !st.IsEnrolled() {
		t.Error("state with NodeID + existing cert/key files should be enrolled")
	}

	// Missing cert file.
	st.CertPath = filepath.Join(dir, "nonexistent.pem")
	if st.IsEnrolled() {
		t.Error("state with missing cert file should not be enrolled")
	}
}

func TestSavePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	st, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	st.NodeID = "test-node"

	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("state file permissions: got %04o, want 0600", perm)
	}
}

func TestCorruptStateReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Write garbage.
	if err := os.WriteFile(path, []byte("not json {{{"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := LoadState(path)
	if err == nil {
		t.Error("LoadState on corrupt file should return an error")
	}
}
