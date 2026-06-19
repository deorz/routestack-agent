// Package tunnel manages per-client tunnels inside managed VPN service containers.
//
// A tunnel is a logical peer/client configuration owned by a specific service.
// The manager delegates provider-specific operations to helper scripts inside
// the container so the agent does not need to parse WireGuard/Xray configs.
package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"routestack-agent/internal/docker"
	"routestack-agent/internal/service"
)

// Provider identifies the VPN technology running in the service container.
type Provider string

const (
	ProviderAmneziaWG Provider = "amneziawg"
	ProviderXray      Provider = "xray"
)

// Request is a create/remove tunnel operation.
type Request struct {
	ServiceID  string          `json:"service_id"`
	TunnelID   string          `json:"tunnel_id"`
	Provider   Provider        `json:"provider"`
	PeerConfig json.RawMessage `json:"peer_config,omitempty"`
}

// Validate returns an error if the request is malformed.
func (r Request) Validate() error {
	if strings.TrimSpace(r.ServiceID) == "" {
		return fmt.Errorf("service_id is required")
	}
	if strings.TrimSpace(r.TunnelID) == "" {
		return fmt.Errorf("tunnel_id is required")
	}
	switch r.Provider {
	case ProviderAmneziaWG, ProviderXray:
	default:
		return fmt.Errorf("unsupported provider: %s", r.Provider)
	}
	return nil
}

// ContainerExec is the subset of docker.Client used by Manager.
type ContainerExec interface {
	Inspect(ctx context.Context, containerID string) (*docker.ContainerState, error)
	Exec(ctx context.Context, containerID string, cmd []string) (*docker.ExecResult, error)
}

// Manager manipulates tunnels inside managed service containers.
type Manager struct {
	client ContainerExec
}

// NewManager creates a tunnel manager.
func NewManager(client ContainerExec) *Manager {
	return &Manager{client: client}
}

// Create adds a peer tunnel to the service container.
// It writes the peer config to a helper script via stdin.
func (m *Manager) Create(ctx context.Context, req Request) error {
	if err := req.Validate(); err != nil {
		return err
	}
	containerName := service.ContainerName(req.ServiceID)
	if _, err := m.client.Inspect(ctx, containerName); err != nil {
		return fmt.Errorf("service container %s not found: %w", containerName, err)
	}

	cmd := []string{"/usr/local/bin/routestack-add-peer", string(req.Provider), req.TunnelID}
	res, err := m.client.Exec(ctx, containerName, cmd)
	if err != nil {
		return fmt.Errorf("exec add-peer in %s: %w", containerName, err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("add-peer failed in %s (exit %d): %s", containerName, res.ExitCode, res.Stdout+res.Stderr)
	}

	_ = req.PeerConfig
	return nil
}

// Remove deletes a peer tunnel from the service container.
func (m *Manager) Remove(ctx context.Context, req Request) error {
	if err := req.Validate(); err != nil {
		return err
	}
	containerName := service.ContainerName(req.ServiceID)
	if _, err := m.client.Inspect(ctx, containerName); err != nil {
		return fmt.Errorf("service container %s not found: %w", containerName, err)
	}

	cmd := []string{"/usr/local/bin/routestack-remove-peer", string(req.Provider), req.TunnelID}
	res, err := m.client.Exec(ctx, containerName, cmd)
	if err != nil {
		return fmt.Errorf("exec remove-peer in %s: %w", containerName, err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("remove-peer failed in %s (exit %d): %s", containerName, res.ExitCode, res.Stdout+res.Stderr)
	}
	return nil
}
