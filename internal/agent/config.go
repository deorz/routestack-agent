package agent

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"routestack-agent/internal/filesystem"
)

// Config is the top-level agent configuration loaded from config.yaml.
type Config struct {
	Agent      AgentConfig      `yaml:"agent"`
	Controller ControllerConfig `yaml:"controller"`
	Paths      PathsConfig      `yaml:"paths"`
	System     SystemConfig     `yaml:"system"`
}

// AgentConfig holds agent identity settings.
type AgentConfig struct {
	Version string `yaml:"version"`
	NodeID  string `yaml:"node_id"`
}

// ControllerConfig holds the control plane connection parameters.
type ControllerConfig struct {
	URL                     string `yaml:"url"`
	CACertPath              string `yaml:"ca_cert_path"`
	HeartbeatIntervalSec    int    `yaml:"heartbeat_interval_sec"`
	OperationPollTimeoutSec int    `yaml:"operation_poll_timeout_sec"`
	ConnectTimeoutSec       int    `yaml:"connect_timeout_sec"`
	RequestTimeoutSec       int    `yaml:"request_timeout_sec"`
}

// PathsConfig allows overriding the default filesystem layout.
// Empty fields fall back to filesystem package constants.
type PathsConfig struct {
	ConfigDir      string `yaml:"config_dir"`
	BackupDir      string `yaml:"backup_dir"`
	StateFile      string `yaml:"state_file"`
	LogFile        string `yaml:"log_file"`
	NftablesConfig string `yaml:"nftables_config"`
}

// SystemConfig holds host-specific settings the agent needs to know.
type SystemConfig struct {
	SSHPort         int    `yaml:"ssh_port"`
	PublicInterface string `yaml:"public_interface"`
}

// LoadConfig reads and validates a YAML config file at path.
// Missing optional fields are populated with defaults.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	if err := cfg.applyDefaults(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	return &cfg, nil
}

// applyDefaults fills zero-value fields with sensible defaults
// and validates that required fields are present.
func (c *Config) applyDefaults() error {
	if c.Controller.URL == "" {
		return fmt.Errorf("controller.url is required")
	}

	if c.Agent.Version == "" {
		c.Agent.Version = "0.1.0"
	}

	if c.Controller.HeartbeatIntervalSec <= 0 {
		c.Controller.HeartbeatIntervalSec = 30
	}
	if c.Controller.OperationPollTimeoutSec <= 0 {
		c.Controller.OperationPollTimeoutSec = 30
	}
	if c.Controller.ConnectTimeoutSec <= 0 {
		c.Controller.ConnectTimeoutSec = 10
	}
	if c.Controller.RequestTimeoutSec <= 0 {
		c.Controller.RequestTimeoutSec = 60
	}
	if c.System.SSHPort <= 0 {
		c.System.SSHPort = 22
	}

	// Path defaults: use filesystem constants for each empty field.
	if c.Paths.ConfigDir == "" {
		c.Paths.ConfigDir = filesystem.ServicesDir
	}
	if c.Paths.BackupDir == "" {
		c.Paths.BackupDir = filesystem.BackupDir
	}
	if c.Paths.StateFile == "" {
		c.Paths.StateFile = filesystem.StateFile
	}
	if c.Paths.LogFile == "" {
		c.Paths.LogFile = filesystem.LogDir + "/agent.log"
	}
	if c.Paths.NftablesConfig == "" {
		c.Paths.NftablesConfig = filesystem.NftablesDir + "/routestack.nft"
	}

	return nil
}
