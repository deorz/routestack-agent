package executor

import (
	"context"
	"encoding/json"
	"fmt"

	"routestack-agent/internal/components"
	"routestack-agent/internal/docker"
	"routestack-agent/internal/installer"
	"routestack-agent/internal/service"
)

// extractContainerName parses and validates container_name from operation payload.
func extractContainerName(data []byte) (string, error) {
	var p struct {
		ContainerName string `json:"container_name"`
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return "", fmt.Errorf("invalid payload: %w", err)
	}
	if p.ContainerName == "" {
		return "", fmt.Errorf("container_name is required")
	}
	return p.ContainerName, nil
}

// findAndAdopt locates a container by exact name or known adoptable names
// and labels it as routestack-managed if it was not created by the agent.
func findAndAdopt(ctx context.Context, client *docker.Client, name string) (string, error) {
	id, err := client.FindContainer(ctx, name)
	if err != nil {
		return "", fmt.Errorf("find container: %w", err)
	}
	if id == "" {
		return "", nil
	}
	// Label pre-existing containers (e.g. deployed by the AmneziaVPN app).
	_ = client.Adopt(ctx, id)
	return id, nil
}

// ── Docker-backed handlers ───────────────────────────────────────────────────

// NewDockerStartHandler returns a handler that starts a Docker container.
// It finds the container by exact name or by known adoptable names, adopts
// it if needed, and starts it.
func NewDockerStartHandler(client *docker.Client) Handler {
	return func(ctx context.Context, op Operation) (*Result, error) {
		containerName, err := extractContainerName(op.Data)
		if err != nil {
			return nil, err
		}

		containerID, err := findAndAdopt(ctx, client, containerName)
		if err != nil {
			return nil, err
		}
		if containerID == "" {
			return &Result{Status: "failed", Message: "container not found"}, nil
		}

		if startErr := client.Start(ctx, containerID); startErr != nil {
			return &Result{Status: "failed", Message: startErr.Error()}, nil
		}
		return &Result{Status: "success", Message: fmt.Sprintf("started %s", containerName)}, nil
	}
}

// NewDockerStopHandler returns a handler that stops a Docker container.
func NewDockerStopHandler(client *docker.Client) Handler {
	return func(ctx context.Context, op Operation) (*Result, error) {
		containerName, err := extractContainerName(op.Data)
		if err != nil {
			return nil, err
		}

		p := struct {
			TimeoutSec int `json:"timeout_sec"`
		}{}
		_ = json.Unmarshal(op.Data, &p)

		containerID, err := findAndAdopt(ctx, client, containerName)
		if err != nil {
			return nil, err
		}
		if containerID == "" {
			return &Result{Status: "failed", Message: "container not found"}, nil
		}

		if stopErr := client.Stop(ctx, containerID, p.TimeoutSec); stopErr != nil {
			return &Result{Status: "failed", Message: stopErr.Error()}, nil
		}
		return &Result{Status: "success", Message: fmt.Sprintf("stopped %s", containerName)}, nil
	}
}

// NewDockerRestartHandler returns a handler that restarts a Docker container.
func NewDockerRestartHandler(client *docker.Client) Handler {
	return func(ctx context.Context, op Operation) (*Result, error) {
		containerName, err := extractContainerName(op.Data)
		if err != nil {
			return nil, err
		}

		p := struct {
			TimeoutSec int `json:"timeout_sec"`
		}{}
		_ = json.Unmarshal(op.Data, &p)

		containerID, err := findAndAdopt(ctx, client, containerName)
		if err != nil {
			return nil, err
		}
		if containerID == "" {
			return &Result{Status: "failed", Message: "container not found"}, nil
		}

		if stopErr := client.Stop(ctx, containerID, p.TimeoutSec); stopErr != nil {
			return &Result{Status: "failed", Message: fmt.Sprintf("stop: %v", stopErr)}, nil
		}
		if startErr := client.Start(ctx, containerID); startErr != nil {
			return &Result{Status: "failed", Message: fmt.Sprintf("start: %v", startErr)}, nil
		}
		return &Result{Status: "success", Message: fmt.Sprintf("restarted %s", containerName)}, nil
	}
}

// ── Stub handlers ────────────────────────────────────────────────────────────

// NewStubHandler returns a handler that returns RedirectError indicating
// which future phase will implement this operation type.
func NewStubHandler(phase string) Handler {
	return func(ctx context.Context, op Operation) (*Result, error) {
		return nil, &RedirectError{OpType: op.Type, Target: phase}
	}
}

// NewInstallComponentHandler returns a handler that pulls and verifies
// a Docker image for a managed component.
func NewInstallComponentHandler(inst *installer.Installer) Handler {
	return func(ctx context.Context, op Operation) (*Result, error) {
		var c components.Component
		if err := json.Unmarshal(op.Data, &c); err != nil {
			return nil, fmt.Errorf("invalid component payload: %w", err)
		}
		if err := inst.Install(ctx, c); err != nil {
			return &Result{Status: "failed", Message: err.Error()}, nil
		}
		return &Result{
			Status:  "success",
			Message: fmt.Sprintf("installed %s (%s)", c.Name, c.Version),
		}, nil
	}
}

// NewApplyServiceRevisionHandler returns a handler that reconciles a managed
// service container with the requested revision spec.
func NewApplyServiceRevisionHandler(mgr *service.Manager) Handler {
	return func(ctx context.Context, op Operation) (*Result, error) {
		var p struct {
			ServiceID string       `json:"service_id"`
			Revision  int64        `json:"revision"`
			Spec      service.Spec `json:"spec"`
		}
		if err := json.Unmarshal(op.Data, &p); err != nil {
			return nil, fmt.Errorf("invalid service revision payload: %w", err)
		}
		if p.ServiceID == "" {
			return nil, fmt.Errorf("service_id is required")
		}
		if err := mgr.ApplyRevision(ctx, p.ServiceID, p.Revision, p.Spec); err != nil {
			return &Result{Status: "failed", Message: err.Error()}, nil
		}
		return &Result{
			Status:  "success",
			Message: fmt.Sprintf("applied revision %d to %s", p.Revision, p.ServiceID),
		}, nil
	}
}

// NewCollectStatusHandler returns a handler that reports container status.
func NewCollectStatusHandler(mgr *service.Manager) Handler {
	return func(ctx context.Context, op Operation) (*Result, error) {
		var p struct {
			ServiceID string `json:"service_id"`
		}
		if err := json.Unmarshal(op.Data, &p); err != nil {
			return nil, fmt.Errorf("invalid status payload: %w", err)
		}
		if p.ServiceID == "" {
			return nil, fmt.Errorf("service_id is required")
		}
		status, err := mgr.CollectStatus(ctx, p.ServiceID)
		if err != nil {
			return &Result{Status: "failed", Message: err.Error()}, nil
		}
		data, _ := json.Marshal(status)
		return &Result{Status: "success", Data: data}, nil
	}
}

// NewCollectLogsHandler returns a handler that returns container logs.
func NewCollectLogsHandler(mgr *service.Manager) Handler {
	return func(ctx context.Context, op Operation) (*Result, error) {
		var p struct {
			ServiceID string `json:"service_id"`
			Tail      int    `json:"tail"`
		}
		if err := json.Unmarshal(op.Data, &p); err != nil {
			return nil, fmt.Errorf("invalid logs payload: %w", err)
		}
		if p.ServiceID == "" {
			return nil, fmt.Errorf("service_id is required")
		}
		logs, err := mgr.CollectLogs(ctx, p.ServiceID, p.Tail)
		if err != nil {
			return &Result{Status: "failed", Message: err.Error()}, nil
		}
		return &Result{Status: "success", Message: logs}, nil
	}
}
