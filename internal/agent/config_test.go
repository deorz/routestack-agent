package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigValid(t *testing.T) {
	yaml := `
agent:
  version: "1.2.3"
  node_id: "test-node"

controller:
  url: "https://panel.example.com"
  ca_cert_path: "/etc/routestack/agent/ca.pem"
  heartbeat_interval_sec: 45
  operation_poll_timeout_sec: 60
  connect_timeout_sec: 15
  request_timeout_sec: 90

paths:
  config_dir: "/custom/services"
  backup_dir: "/custom/backups"
  state_file: "/custom/state.json"
  log_file: "/custom/agent.log"
  nftables_config: "/custom/routestack.nft"

system:
  ssh_port: 2222
  public_interface: "eth0"
`
	path := writeTempYAML(t, yaml)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Agent.Version != "1.2.3" {
		t.Errorf("Version: got %q, want %q", cfg.Agent.Version, "1.2.3")
	}
	if cfg.Controller.URL != "https://panel.example.com" {
		t.Errorf("URL: got %q", cfg.Controller.URL)
	}
	if cfg.Controller.HeartbeatIntervalSec != 45 {
		t.Errorf("HeartbeatIntervalSec: got %d, want 45", cfg.Controller.HeartbeatIntervalSec)
	}
	if cfg.Paths.ConfigDir != "/custom/services" {
		t.Errorf("ConfigDir: got %q, want /custom/services", cfg.Paths.ConfigDir)
	}
	if cfg.System.SSHPort != 2222 {
		t.Errorf("SSHPort: got %d, want 2222", cfg.System.SSHPort)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	yaml := `
controller:
  url: "https://panel.example.com"
`
	path := writeTempYAML(t, yaml)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// Check defaults are applied.
	if cfg.Agent.Version != "0.1.0" {
		t.Errorf("Version default: got %q, want 0.1.0", cfg.Agent.Version)
	}
	if cfg.Controller.HeartbeatIntervalSec != 30 {
		t.Errorf("HeartbeatIntervalSec default: got %d, want 30", cfg.Controller.HeartbeatIntervalSec)
	}
	if cfg.Controller.OperationPollTimeoutSec != 30 {
		t.Errorf("OperationPollTimeoutSec default: got %d, want 30", cfg.Controller.OperationPollTimeoutSec)
	}
	if cfg.Controller.ConnectTimeoutSec != 10 {
		t.Errorf("ConnectTimeoutSec default: got %d, want 10", cfg.Controller.ConnectTimeoutSec)
	}
	if cfg.Controller.RequestTimeoutSec != 60 {
		t.Errorf("RequestTimeoutSec default: got %d, want 60", cfg.Controller.RequestTimeoutSec)
	}
	if cfg.System.SSHPort != 22 {
		t.Errorf("SSHPort default: got %d, want 22", cfg.System.SSHPort)
	}

	// Path defaults should be populated from filesystem package.
	if cfg.Paths.ConfigDir == "" {
		t.Error("Paths.ConfigDir should have a default")
	}
	if cfg.Paths.BackupDir == "" {
		t.Error("Paths.BackupDir should have a default")
	}
	if cfg.Paths.StateFile == "" {
		t.Error("Paths.StateFile should have a default")
	}
	if cfg.Paths.LogFile == "" {
		t.Error("Paths.LogFile should have a default")
	}
	if cfg.Paths.NftablesConfig == "" {
		t.Error("Paths.NftablesConfig should have a default")
	}
}

func TestLoadConfigMissingURL(t *testing.T) {
	yaml := `
agent:
  version: "1.0.0"
`
	path := writeTempYAML(t, yaml)

	_, err := LoadConfig(path)
	if err == nil {
		t.Error("LoadConfig without controller.url should return an error")
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("LoadConfig on nonexistent file should return an error")
	}
}

func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}
