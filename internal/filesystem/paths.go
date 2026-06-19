// Package filesystem defines canonical filesystem paths and directory constants
// used across the agent. All packages reference these constants instead of
// hardcoding paths, ensuring consistency between config defaults and runtime.
package filesystem

const (
	ConfigDir      = "/etc/routestack"
	ServicesDir    = "/etc/routestack/services"
	NftablesDir    = "/etc/routestack/nftables"
	AgentConfigDir = "/etc/routestack/agent"
	BackupDir      = "/var/lib/routestack/backups"
	StateFile      = "/var/lib/routestack/state.json"
	ComponentsDir  = "/var/lib/routestack/components"
	CertsDir       = "/etc/letsencrypt"
	LockFilePath   = "/etc/routestack/components.lock.yaml"
	SystemdDir     = "/etc/systemd/system"
	LogDir         = "/var/log/routestack"
	RunDir         = "/run/routestack"
)

// DefaultCertPath is where the agent stores its client certificate after enrollment.
const DefaultCertPath = ConfigDir + "/agent/cert.pem"

// DefaultKeyPath is where the agent stores its private key after enrollment.
const DefaultKeyPath = ConfigDir + "/agent/key.pem"

// AllowedWritePrefixes lists directories (and files) the agent is permitted
// to write to. The executor and installer packages use this to validate
// write targets before touching disk.
var AllowedWritePrefixes = []string{
	ServicesDir,
	NftablesDir,
	BackupDir,
	StateFile,
	ComponentsDir,
	CertsDir,
	SystemdDir, // only routestack-* prefixed units
	LogDir,
	RunDir,
}
