package tunnel

import (
	"context"
	"errors"
	"testing"

	"routestack-agent/internal/docker"
)

type mockExecClient struct {
	state      *docker.ContainerState
	inspectErr error
	execs      [][]string
	results    []*docker.ExecResult
	execErr    error
	execIndex  int
}

func (m *mockExecClient) Inspect(_ context.Context, _ string) (*docker.ContainerState, error) {
	return m.state, m.inspectErr
}

func (m *mockExecClient) Exec(_ context.Context, _ string, cmd []string) (*docker.ExecResult, error) {
	m.execs = append(m.execs, cmd)
	idx := m.execIndex
	m.execIndex++
	if m.execErr != nil {
		return nil, m.execErr
	}
	if idx < len(m.results) {
		return m.results[idx], nil
	}
	return &docker.ExecResult{}, nil
}

func TestValidateRequest(t *testing.T) {
	if err := (Request{ServiceID: "svc", TunnelID: "tun", Provider: ProviderAmneziaWG}).Validate(); err != nil {
		t.Errorf("valid request rejected: %v", err)
	}

	invalid := []struct {
		name string
		req  Request
		want string
	}{
		{"missing service_id", Request{TunnelID: "tun", Provider: ProviderXray}, "service_id"},
		{"missing tunnel_id", Request{ServiceID: "svc", Provider: ProviderXray}, "tunnel_id"},
		{"missing provider", Request{ServiceID: "svc", TunnelID: "tun"}, "unsupported provider"},
		{"bad provider", Request{ServiceID: "svc", TunnelID: "tun", Provider: "wireguard"}, "unsupported provider"},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if err == nil || err.Error()[:len(tc.want)] != tc.want {
				t.Errorf("expected error starting with %q, got %v", tc.want, err)
			}
		})
	}
}

func TestCreateTunnel(t *testing.T) {
	client := &mockExecClient{
		state: &docker.ContainerState{},
		results: []*docker.ExecResult{
			{ExitCode: 0, Stdout: "ok"},
		},
	}
	mgr := NewManager(client)

	req := Request{ServiceID: "vpn", TunnelID: "peer-1", Provider: ProviderAmneziaWG}
	if err := mgr.Create(context.Background(), req); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if len(client.execs) != 1 {
		t.Fatalf("expected 1 exec, got %v", client.execs)
	}
	cmd := client.execs[0]
	if len(cmd) != 3 || cmd[0] != "/usr/local/bin/routestack-add-peer" || cmd[1] != "amneziawg" || cmd[2] != "peer-1" {
		t.Errorf("unexpected cmd: %v", cmd)
	}
}

func TestCreateTunnelContainerNotFound(t *testing.T) {
	client := &mockExecClient{inspectErr: errors.New("not found")}
	mgr := NewManager(client)

	req := Request{ServiceID: "vpn", TunnelID: "peer-1", Provider: ProviderAmneziaWG}
	if err := mgr.Create(context.Background(), req); err == nil {
		t.Fatal("expected error when container missing")
	}
}

func TestCreateTunnelExecFailure(t *testing.T) {
	client := &mockExecClient{
		state: &docker.ContainerState{},
		results: []*docker.ExecResult{
			{ExitCode: 1, Stdout: "no such script"},
		},
	}
	mgr := NewManager(client)

	req := Request{ServiceID: "vpn", TunnelID: "peer-1", Provider: ProviderXray}
	if err := mgr.Create(context.Background(), req); err == nil {
		t.Fatal("expected error on exec failure")
	}
}

func TestRemoveTunnel(t *testing.T) {
	client := &mockExecClient{
		state: &docker.ContainerState{},
		results: []*docker.ExecResult{
			{ExitCode: 0, Stdout: "removed"},
		},
	}
	mgr := NewManager(client)

	req := Request{ServiceID: "vpn", TunnelID: "peer-1", Provider: ProviderXray}
	if err := mgr.Remove(context.Background(), req); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if len(client.execs) != 1 {
		t.Fatalf("expected 1 exec, got %v", client.execs)
	}
	cmd := client.execs[0]
	if len(cmd) != 3 || cmd[0] != "/usr/local/bin/routestack-remove-peer" || cmd[1] != "xray" || cmd[2] != "peer-1" {
		t.Errorf("unexpected cmd: %v", cmd)
	}
}
