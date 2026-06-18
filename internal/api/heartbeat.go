package api

import (
	"context"
	"os"
	"os/exec"
	"runtime"
)

// HeartbeatRequest is the payload sent periodically to the controller.
type HeartbeatRequest struct {
	NodeID        string          `json:"node_id"`
	AgentVersion  string          `json:"agent_version"`
	OS            string          `json:"os"`
	Architecture  string          `json:"architecture"`
	KernelVersion string          `json:"kernel_version"`
	Capabilities  []string        `json:"capabilities"`
	Services      []ServiceStatus `json:"services"`
	System        SystemMetrics   `json:"system"`
}

// ServiceStatus reports the current state of a managed service.
type ServiceStatus struct {
	ServiceID      string `json:"service_id"`
	Type           string `json:"type"`
	ActualState    string `json:"actual_state"`
	ActiveRevision int    `json:"active_revision"`
	PID            int    `json:"pid"`
	PortListening  bool   `json:"port_listening"`
}

// SystemMetrics holds lightweight system health data.
type SystemMetrics struct {
	CPUPercent   float64 `json:"cpu_percent"`
	MemoryUsedMB int64   `json:"memory_used_mb"`
	DiskFreeGB   float64 `json:"disk_free_gb"`
	LoadAvg1m    float64 `json:"load_avg_1m"`
}

// DetectCapabilities probes the system for available VPN/proxy components.
func DetectCapabilities() []string {
	var caps []string

	for _, bin := range []string{
		"xray",
		"amnezia-wg",
		"amnezia-wg-go",
		"haproxy",
		"3proxy",
		"telemt",
		"mtproto-proxy",
		"nft",
	} {
		if _, err := exec.LookPath(bin); err == nil {
			caps = append(caps, bin)
		}
	}

	// Check for kernel modules.
	if _, err := os.Stat("/sys/module/amneziawg"); err == nil {
		caps = append(caps, "amneziawg-kernel")
	}
	if _, err := os.Stat("/sys/module/nf_tables"); err == nil {
		caps = append(caps, "nftables-kernel")
	}

	return caps
}

// Heartbeat sends a heartbeat to the controller and returns the response.
func Heartbeat(ctx context.Context, client *Client, req HeartbeatRequest) (*HeartbeatResponse, error) {
	return client.Heartbeat(ctx, req)
}

// Ensure runtime import is used (GOARCH in platform files).
var _ = runtime.GOARCH
