package executor

import (
	"context"
	"encoding/json"
	"fmt"

	"routestack-agent/internal/docker"
)

// ── Docker-backed handlers ───────────────────────────────────────────────────

// NewDockerStartHandler returns a handler that starts a Docker container.
func NewDockerStartHandler(client *docker.Client) Handler {
	return func(ctx context.Context, op Operation) (*Result, error) {
		var p struct {
			ContainerName string `json:"container_name"`
		}
		if err := json.Unmarshal(op.Data, &p); err != nil {
			return nil, fmt.Errorf("invalid payload: %w", err)
		}
		if p.ContainerName == "" {
			return nil, fmt.Errorf("container_name is required")
		}

		if err := client.Start(ctx, p.ContainerName); err != nil {
			return &Result{Status: "failed", Message: err.Error()}, nil
		}
		return &Result{Status: "success", Message: fmt.Sprintf("started %s", p.ContainerName)}, nil
	}
}

// NewDockerStopHandler returns a handler that stops a Docker container.
func NewDockerStopHandler(client *docker.Client) Handler {
	return func(ctx context.Context, op Operation) (*Result, error) {
		var p struct {
			ContainerName string `json:"container_name"`
			TimeoutSec    int    `json:"timeout_sec"`
		}
		if err := json.Unmarshal(op.Data, &p); err != nil {
			return nil, fmt.Errorf("invalid payload: %w", err)
		}
		if p.ContainerName == "" {
			return nil, fmt.Errorf("container_name is required")
		}

		if err := client.Stop(ctx, p.ContainerName, p.TimeoutSec); err != nil {
			return &Result{Status: "failed", Message: err.Error()}, nil
		}
		return &Result{Status: "success", Message: fmt.Sprintf("stopped %s", p.ContainerName)}, nil
	}
}

// NewDockerRestartHandler returns a handler that restarts a Docker container
// (stop then start).
func NewDockerRestartHandler(client *docker.Client) Handler {
	return func(ctx context.Context, op Operation) (*Result, error) {
		var p struct {
			ContainerName string `json:"container_name"`
			TimeoutSec    int    `json:"timeout_sec"`
		}
		if err := json.Unmarshal(op.Data, &p); err != nil {
			return nil, fmt.Errorf("invalid payload: %w", err)
		}
		if p.ContainerName == "" {
			return nil, fmt.Errorf("container_name is required")
		}

		if err := client.Stop(ctx, p.ContainerName, p.TimeoutSec); err != nil {
			return &Result{Status: "failed", Message: fmt.Sprintf("stop: %v", err)}, nil
		}
		if err := client.Start(ctx, p.ContainerName); err != nil {
			return &Result{Status: "failed", Message: fmt.Sprintf("start: %v", err)}, nil
		}
		return &Result{Status: "success", Message: fmt.Sprintf("restarted %s", p.ContainerName)}, nil
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
