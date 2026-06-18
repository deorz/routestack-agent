// Package state provides persistent agent state backed by a JSON file.
//
// State is loaded at startup and saved atomically (write temp → rename)
// whenever enrollment data or revision tracking changes.
// The state file NEVER contains secrets — certs and keys are stored
// separately on disk, only their paths are recorded here.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// AgentState is the persistent on-disk state of the agent.
type AgentState struct {
	NodeID             string         `json:"node_id"`
	CertPath           string         `json:"cert_path"`
	KeyPath            string         `json:"key_path"`
	LastKnownRevisions map[string]int `json:"last_known_revisions"`

	path string // filesystem path for Save()
}

// LoadState reads the state file at path. If the file does not exist,
// it returns an empty state backed by that path (no error).
// If the file exists but cannot be parsed, it returns an error.
func LoadState(path string) (*AgentState, error) {
	s := &AgentState{
		LastKnownRevisions: make(map[string]int),
		path:               path,
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("read state file %s: %w", path, err)
	}

	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("parse state file %s: %w", path, err)
	}

	// Ensure maps are non-nil after unmarshal.
	if s.LastKnownRevisions == nil {
		s.LastKnownRevisions = make(map[string]int)
	}
	s.path = path
	return s, nil
}

// Save writes the state atomically to the file specified at load time.
// It marshals to JSON with indentation, writes to a temp file in the same
// directory, then renames — ensuring the on-disk state is never partial.
// Permissions are set to 0600.
func (s *AgentState) Save() error {
	if s.path == "" {
		return errors.New("state: no path set — was LoadState called?")
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("state: marshal: %w", err)
	}

	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".routestack-state-*.tmp")
	if err != nil {
		return fmt.Errorf("state: create temp: %w", err)
	}
	tmpPath := tmp.Name()

	// Write + close explicitly to catch write errors.
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("state: write temp: %w", err)
	}
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("state: chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("state: close temp: %w", err)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("state: rename temp → %s: %w", s.path, err)
	}
	return nil
}

// IsEnrolled returns true when the agent has completed enrollment
// and the cert+key files actually exist on disk.
func (s *AgentState) IsEnrolled() bool {
	if s.NodeID == "" || s.CertPath == "" || s.KeyPath == "" {
		return false
	}
	if _, err := os.Stat(s.CertPath); err != nil {
		return false
	}
	if _, err := os.Stat(s.KeyPath); err != nil {
		return false
	}
	return true
}
