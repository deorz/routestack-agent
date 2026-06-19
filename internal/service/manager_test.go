package service

import (
	"context"
	"errors"
	"testing"

	"routestack-agent/internal/docker"
)

type mockContainerClient struct {
	inspectState *docker.ContainerState
	inspectErr   error
	created      []docker.ContainerSpec
	createID     string
	createErr    error
	started      []string
	startErr     error
	stopped      []string
	removed      []string
	logs         string
	logsErr      error
}

func (m *mockContainerClient) Inspect(_ context.Context, _ string) (*docker.ContainerState, error) {
	return m.inspectState, m.inspectErr
}

func (m *mockContainerClient) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	m.created = append(m.created, spec)
	return m.createID, m.createErr
}

func (m *mockContainerClient) Start(_ context.Context, id string) error {
	m.started = append(m.started, id)
	return m.startErr
}

func (m *mockContainerClient) Stop(_ context.Context, id string, _ int) error {
	m.stopped = append(m.stopped, id)
	return nil
}

func (m *mockContainerClient) Remove(_ context.Context, id string, _ bool) error {
	m.removed = append(m.removed, id)
	return nil
}

func (m *mockContainerClient) Logs(_ context.Context, _ string, _ int) (string, error) {
	return m.logs, m.logsErr
}

func TestContainerName(t *testing.T) {
	if got := ContainerName("vpn-1"); got != "routestack-vpn-1" {
		t.Errorf("ContainerName = %q", got)
	}
}

func TestSpecToContainerSpec(t *testing.T) {
	s := Spec{
		Image:   "alpine:3.18",
		Command: []string{"sh", "-c", "sleep 30"},
		Ports:   []Port{{HostPort: "8080", ContainerPort: "80"}},
		Volumes: []Volume{{Source: "/data", Target: "/data", Mode: "ro"}},
		Env:     map[string]string{"FOO": "bar"},
		Restart: "always",
	}
	spec := s.ToContainerSpec("my-svc")
	if spec.Name != "routestack-my-svc" {
		t.Errorf("Name = %q", spec.Name)
	}
	if spec.Image != "alpine:3.18" {
		t.Errorf("Image = %q", spec.Image)
	}
	if len(spec.Cmd) != 3 || spec.Cmd[2] != "sleep 30" {
		t.Errorf("Cmd = %v", spec.Cmd)
	}
	if len(spec.Ports) != 1 || spec.Ports[0].Protocol != "tcp" {
		t.Errorf("Ports = %v", spec.Ports)
	}
	if len(spec.Volumes) != 1 || spec.Volumes[0].Mode != "ro" {
		t.Errorf("Volumes = %v", spec.Volumes)
	}
	if len(spec.Env) != 1 || spec.Env[0] != "FOO=bar" {
		t.Errorf("Env = %v", spec.Env)
	}
	if spec.Restart != "always" {
		t.Errorf("Restart = %q", spec.Restart)
	}
}

func TestApplyRevisionCreatesContainer(t *testing.T) {
	client := &mockContainerClient{
		inspectErr: errors.New("not found"),
		createID:   "abc123",
	}
	mgr := NewManager(client)

	spec := Spec{Image: "alpine:3.18"}
	if err := mgr.ApplyRevision(context.Background(), "svc-1", 1, spec); err != nil {
		t.Fatalf("ApplyRevision failed: %v", err)
	}
	if len(client.created) != 1 {
		t.Fatalf("created %d containers, want 1", len(client.created))
	}
	if client.created[0].Name != "routestack-svc-1" {
		t.Errorf("created name = %q", client.created[0].Name)
	}
	if len(client.started) != 1 || client.started[0] != "abc123" {
		t.Errorf("started = %v", client.started)
	}
}

func TestApplyRevisionRemovesExisting(t *testing.T) {
	client := &mockContainerClient{
		inspectState: &docker.ContainerState{ID: "old-id"},
		createID:     "new-id",
	}
	mgr := NewManager(client)

	spec := Spec{Image: "alpine:3.19"}
	if err := mgr.ApplyRevision(context.Background(), "svc-1", 2, spec); err != nil {
		t.Fatalf("ApplyRevision failed: %v", err)
	}
	if len(client.stopped) != 1 || client.stopped[0] != "old-id" {
		t.Errorf("stopped = %v", client.stopped)
	}
	if len(client.removed) != 1 || client.removed[0] != "old-id" {
		t.Errorf("removed = %v", client.removed)
	}
	if len(client.created) != 1 {
		t.Fatalf("created %d containers, want 1", len(client.created))
	}
}

func TestCollectStatus(t *testing.T) {
	client := &mockContainerClient{
		inspectState: &docker.ContainerState{
			ID:     "cid-1",
			State:  docker.ContainerState{}.State,
			Config: docker.ContainerConfig{Image: "alpine:3.18", Labels: map[string]string{"managed-by": "routestack"}},
		},
	}
	client.inspectState.State.Running = true
	client.inspectState.State.Status = "running"

	mgr := NewManager(client)
	status, err := mgr.CollectStatus(context.Background(), "svc-1")
	if err != nil {
		t.Fatalf("CollectStatus failed: %v", err)
	}
	if status.ContainerName != "routestack-svc-1" {
		t.Errorf("ContainerName = %q", status.ContainerName)
	}
	if !status.Running {
		t.Error("expected running")
	}
	if !status.ManagedByAgent {
		t.Error("expected managed by agent")
	}
}

func TestCollectLogs(t *testing.T) {
	client := &mockContainerClient{logs: "line1\nline2\n"}
	mgr := NewManager(client)

	logs, err := mgr.CollectLogs(context.Background(), "svc-1", 50)
	if err != nil {
		t.Fatalf("CollectLogs failed: %v", err)
	}
	if logs != client.logs {
		t.Errorf("logs = %q", logs)
	}
}
