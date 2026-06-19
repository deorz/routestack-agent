// Package health runs health checks against managed service containers.
package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"routestack-agent/internal/docker"
	"routestack-agent/internal/service"
)

// ContainerInspector is the subset of docker.Client used by Manager.
type ContainerInspector interface {
	Inspect(ctx context.Context, containerID string) (*docker.ContainerState, error)
}

// Manager runs health checks on managed containers.
type Manager struct {
	inspector ContainerInspector
}

// NewManager creates a health check manager.
func NewManager(inspector ContainerInspector) *Manager {
	return &Manager{inspector: inspector}
}

// Report is the result of a health check.
type Report struct {
	ServiceID     string    `json:"service_id"`
	ContainerName string    `json:"container_name"`
	ContainerID   string    `json:"container_id"`
	Healthy       bool      `json:"healthy"`
	Running       bool      `json:"running"`
	Status        string    `json:"status"`
	Health        string    `json:"health,omitempty"`
	ExitCode      int       `json:"exit_code"`
	PortChecks    []PortHit `json:"port_checks,omitempty"`
	CheckedAt     time.Time `json:"checked_at"`
}

// PortHit reports whether a host port was reachable.
type PortHit struct {
	HostPort string `json:"host_port"`
	Open     bool   `json:"open"`
}

// CheckOptions configures optional probe behavior.
type CheckOptions struct {
	ProbePorts []string `json:"probe_ports,omitempty"`
	TimeoutMs  int      `json:"timeout_ms,omitempty"`
}

// Check inspects the service container and optionally probes host ports.
func (m *Manager) Check(ctx context.Context, serviceID string, opts CheckOptions) (*Report, error) {
	containerName := service.ContainerName(serviceID)
	state, err := m.inspector.Inspect(ctx, containerName)
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", containerName, err)
	}

	report := &Report{
		ServiceID:     serviceID,
		ContainerName: containerName,
		ContainerID:   state.ID,
		Running:       state.State.Running,
		Status:        state.State.Status,
		ExitCode:      state.State.ExitCode,
		CheckedAt:     time.Now().UTC(),
		Healthy:       state.State.Running,
	}
	if state.State.Health != nil {
		report.Health = state.State.Health.Status
		report.Healthy = report.Healthy && report.Health == "healthy"
	}

	timeout := 2 * time.Second
	if opts.TimeoutMs > 0 {
		timeout = time.Duration(opts.TimeoutMs) * time.Millisecond
	}

	for _, port := range opts.ProbePorts {
		open := probePort(ctx, port, timeout)
		report.PortChecks = append(report.PortChecks, PortHit{HostPort: port, Open: open})
		if !open {
			report.Healthy = false
		}
	}

	return report, nil
}

// CheckFromJSON parses options from raw JSON and runs a check.
func (m *Manager) CheckFromJSON(ctx context.Context, serviceID string, raw []byte) (*Report, error) {
	var opts CheckOptions
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &opts)
	}
	return m.Check(ctx, serviceID, opts)
}

func probePort(ctx context.Context, port string, timeout time.Duration) bool {
	if !strings.Contains(port, ":") {
		port = "127.0.0.1:" + port
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", port)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// ParsePort parses a host port string, returning the numeric port.
func ParsePort(s string) (int, error) {
	_, portStr, err := net.SplitHostPort(s)
	if err != nil {
		portStr = s
	}
	return strconv.Atoi(portStr)
}
