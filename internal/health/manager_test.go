package health

import (
	"context"
	"errors"
	"testing"

	"routestack-agent/internal/docker"
)

type mockInspector struct {
	state *docker.ContainerState
	err   error
}

func (m *mockInspector) Inspect(_ context.Context, _ string) (*docker.ContainerState, error) {
	return m.state, m.err
}

func TestCheckRunning(t *testing.T) {
	state := &docker.ContainerState{
		ID: "cid-1",
	}
	state.State.Running = true
	state.State.Status = "running"

	mgr := NewManager(&mockInspector{state: state})
	report, err := mgr.Check(context.Background(), "vpn", CheckOptions{})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !report.Healthy {
		t.Error("expected healthy")
	}
	if report.ContainerName != "routestack-vpn" {
		t.Errorf("ContainerName = %q", report.ContainerName)
	}
}

func TestCheckNotRunning(t *testing.T) {
	state := &docker.ContainerState{ID: "cid-1"}
	state.State.Running = false
	state.State.Status = "exited"
	state.State.ExitCode = 1

	mgr := NewManager(&mockInspector{state: state})
	report, err := mgr.Check(context.Background(), "vpn", CheckOptions{})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if report.Healthy {
		t.Error("expected not healthy")
	}
	if report.ExitCode != 1 {
		t.Errorf("ExitCode = %d", report.ExitCode)
	}
}

func TestCheckWithHealthStatus(t *testing.T) {
	state := &docker.ContainerState{ID: "cid-1"}
	state.State.Running = true
	state.State.Status = "running"
	state.State.Health = &struct {
		Status string `json:"Status"`
	}{Status: "unhealthy"}

	mgr := NewManager(&mockInspector{state: state})
	report, err := mgr.Check(context.Background(), "vpn", CheckOptions{})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if report.Healthy {
		t.Error("expected unhealthy")
	}
	if report.Health != "unhealthy" {
		t.Errorf("Health = %q", report.Health)
	}
}

func TestCheckInspectError(t *testing.T) {
	mgr := NewManager(&mockInspector{err: errors.New("not found")})
	if _, err := mgr.Check(context.Background(), "vpn", CheckOptions{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestProbePortInvalid(t *testing.T) {
	open := probePort(context.Background(), "notaport", 0)
	if open {
		t.Error("expected false for invalid port")
	}
}

func TestParsePort(t *testing.T) {
	if p, err := ParsePort("127.0.0.1:8080"); p != 8080 || err != nil {
		t.Errorf("ParsePort = %d, %v", p, err)
	}
	if p, err := ParsePort("443"); p != 443 || err != nil {
		t.Errorf("ParsePort = %d, %v", p, err)
	}
	if _, err := ParsePort("abc"); err == nil {
		t.Error("expected error")
	}
}
