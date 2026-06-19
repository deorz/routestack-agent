package service

import (
	"context"
	"fmt"

	"routestack-agent/internal/docker"
)

// ContainerClient is the subset of docker.Client used by Manager.
type ContainerClient interface {
	Inspect(ctx context.Context, containerID string) (*docker.ContainerState, error)
	Create(ctx context.Context, spec docker.ContainerSpec) (string, error)
	Start(ctx context.Context, containerID string) error
	Stop(ctx context.Context, containerID string, timeoutSec int) error
	Remove(ctx context.Context, containerID string, force bool) error
	Logs(ctx context.Context, containerID string, tail int) (string, error)
}

// Manager ensures service containers match their desired revision.
type Manager struct {
	client ContainerClient
}

// NewManager creates a service manager.
func NewManager(client ContainerClient) *Manager {
	return &Manager{client: client}
}

// Status holds runtime state for a service container.
type Status struct {
	ServiceID      string `json:"service_id"`
	ContainerName  string `json:"container_name"`
	ContainerID    string `json:"container_id"`
	Running        bool   `json:"running"`
	Status         string `json:"status"`
	ExitCode       int    `json:"exit_code"`
	Health         string `json:"health,omitempty"`
	Image          string `json:"image"`
	ManagedByAgent bool   `json:"managed_by_agent"`
}

// ApplyRevision reconciles the service container with the given spec.
// If a container already exists for the service it is removed and recreated
// so the new revision takes effect. The operation is idempotent at the
// executor layer, so the same revision is not applied twice.
func (m *Manager) ApplyRevision(ctx context.Context, serviceID string, rev int64, spec Spec) error {
	if err := spec.Validate(); err != nil {
		return fmt.Errorf("invalid spec: %w", err)
	}

	containerName := ContainerName(serviceID)

	// Remove existing container for this service if present.
	if existing, err := m.client.Inspect(ctx, containerName); err == nil && existing != nil {
		_ = m.client.Stop(ctx, existing.ID, 10)
		if rmErr := m.client.Remove(ctx, existing.ID, true); rmErr != nil {
			return fmt.Errorf("remove existing container %s: %w", containerName, rmErr)
		}
	}

	containerSpec := spec.ToContainerSpec(serviceID)
	if containerSpec.Name == "" {
		containerSpec.Name = containerName
	}

	id, err := m.client.Create(ctx, containerSpec)
	if err != nil {
		return fmt.Errorf("create container %s: %w", containerName, err)
	}
	if err := m.client.Start(ctx, id); err != nil {
		return fmt.Errorf("start container %s: %w", containerName, err)
	}
	_ = rev
	return nil
}

// CollectStatus returns runtime status for a service container.
func (m *Manager) CollectStatus(ctx context.Context, serviceID string) (*Status, error) {
	containerName := ContainerName(serviceID)
	state, err := m.client.Inspect(ctx, containerName)
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", containerName, err)
	}

	status := &Status{
		ServiceID:     serviceID,
		ContainerName: containerName,
		ContainerID:   state.ID,
		Running:       state.State.Running,
		Status:        state.State.Status,
		ExitCode:      state.State.ExitCode,
		Image:         state.Config.Image,
	}
	if state.State.Health != nil {
		status.Health = state.State.Health.Status
	}
	if state.Config.Labels["managed-by"] == "routestack" {
		status.ManagedByAgent = true
	}

	return status, nil
}

// CollectLogs returns the last tail lines of logs for a service container.
func (m *Manager) CollectLogs(ctx context.Context, serviceID string, tail int) (string, error) {
	containerName := ContainerName(serviceID)
	if tail <= 0 {
		tail = 100
	}
	return m.client.Logs(ctx, containerName, tail)
}
