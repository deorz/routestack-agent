package docker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInspect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/containers/test-container/json") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(ContainerState{
			ID:   "abc123",
			Name: "/test-container",
		})
	}))
	defer srv.Close()
	_ = srv
}

func TestPullError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"image not found"}`))
	}))
	defer srv.Close()
	_ = srv
}

func TestContainerSpecBuildRequest(t *testing.T) {
	c := &Client{apiVersion: "v1.45"}

	spec := ContainerSpec{
		Name:    "routestack-awg-1",
		Image:   "amnezia-awg2:latest",
		Restart: "always",
		Ports: []PortMapping{
			{HostPort: "36584", ContainerPort: "36584", Protocol: "udp"},
		},
		Volumes: []VolumeMount{
			{Source: "/etc/routestack/services/awg", Target: "/opt/amnezia", Mode: "ro"},
		},
		Env: []string{"AWG_CONFIG=/opt/amnezia/config.conf"},
	}

	body := c.buildCreateRequest(spec)

	if body["Image"] != "amnezia-awg2:latest" {
		t.Errorf("Image: got %q", body["Image"])
	}

	labels, ok := body["Labels"].(map[string]string)
	if !ok {
		t.Fatal("Labels missing or wrong type")
	}
	if labels["managed-by"] != "routestack" {
		t.Errorf("managed-by label: got %q", labels["managed-by"])
	}

	hostCfg, ok := body["HostConfig"].(map[string]any)
	if !ok {
		t.Fatal("HostConfig missing")
	}
	restart, ok := hostCfg["RestartPolicy"].(map[string]any)
	if !ok {
		t.Fatal("RestartPolicy missing")
	}
	if restart["Name"] != "always" {
		t.Errorf("RestartPolicy.Name: got %q", restart["Name"])
	}
}

func TestContainerSpecMinimal(t *testing.T) {
	c := &Client{apiVersion: "v1.45"}
	spec := ContainerSpec{
		Name:  "minimal",
		Image: "alpine:latest",
	}
	body := c.buildCreateRequest(spec)

	if body["Image"] != "alpine:latest" {
		t.Errorf("Image: got %q", body["Image"])
	}
	if _, ok := body["HostConfig"]; ok {
		t.Error("HostConfig should be absent for minimal spec")
	}
}

func TestStopMissingContainer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"No such container: missing"}`))
	}))
	defer srv.Close()
	_ = srv
}

func TestContainerInfoUnmarshal(t *testing.T) {
	payload := `[{"Id":"abc","Names":["/xray"],"Image":"xray:latest","State":"running","Status":"Up 2 hours"}]`
	var containers []ContainerInfo
	if err := json.Unmarshal([]byte(payload), &containers); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
	if containers[0].State != "running" {
		t.Errorf("State: got %q", containers[0].State)
	}
}

func TestContainerStateUnmarshal(t *testing.T) {
	payload := `{
		"Id": "abc",
		"Created": "2026-01-01T00:00:00Z",
		"State": {
			"Status": "running",
			"Running": true,
			"ExitCode": 0,
			"StartedAt": "2026-01-01T00:00:01Z",
			"FinishedAt": "0001-01-01T00:00:00Z"
		},
		"Name": "/amnezia-awg"
	}`
	var state ContainerState
	if err := json.Unmarshal([]byte(payload), &state); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !state.State.Running {
		t.Error("expected running")
	}
	if state.Name != "/amnezia-awg" {
		t.Errorf("Name: got %q", state.Name)
	}
}

func TestDockerClientError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"something went wrong"}`))
	}))
	defer srv.Close()
	_ = srv
}
