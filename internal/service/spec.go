// Package service manages routestack service containers.
//
// A service is a logical entity identified by service_id. The manager maps
// each service to a Docker container named "routestack-<service_id>" and
// ensures the running container matches the requested revision spec.
package service

import (
	"fmt"
	"strings"

	"routestack-agent/internal/docker"
)

// Port maps a host port to a container port/protocol.
type Port struct {
	HostPort      string `json:"host_port"`
	ContainerPort string `json:"container_port"`
	Protocol      string `json:"protocol,omitempty"`
}

// Volume maps a host path to a container path.
type Volume struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Mode   string `json:"mode,omitempty"`
}

// Spec describes the desired state of a managed service container.
type Spec struct {
	Image   string            `json:"image"`
	Command []string          `json:"command,omitempty"`
	Ports   []Port            `json:"ports,omitempty"`
	Volumes []Volume          `json:"volumes,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Restart string            `json:"restart,omitempty"`
}

// Validate returns an error if the spec is incomplete.
func (s Spec) Validate() error {
	if strings.TrimSpace(s.Image) == "" {
		return fmt.Errorf("image is required")
	}
	for i, p := range s.Ports {
		if strings.TrimSpace(p.HostPort) == "" || strings.TrimSpace(p.ContainerPort) == "" {
			return fmt.Errorf("port %d: host_port and container_port are required", i)
		}
		if p.Protocol == "" {
			p.Protocol = "tcp"
		}
	}
	for i, v := range s.Volumes {
		if strings.TrimSpace(v.Source) == "" || strings.TrimSpace(v.Target) == "" {
			return fmt.Errorf("volume %d: source and target are required", i)
		}
	}
	return nil
}

// ContainerName returns the Docker container name for a service.
func ContainerName(serviceID string) string {
	return "routestack-" + serviceID
}

// ToContainerSpec converts a service Spec into a docker.ContainerSpec.
func (s Spec) ToContainerSpec(serviceID string) docker.ContainerSpec {
	ports := make([]docker.PortMapping, 0, len(s.Ports))
	for _, p := range s.Ports {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		ports = append(ports, docker.PortMapping{
			HostPort:      p.HostPort,
			ContainerPort: p.ContainerPort,
			Protocol:      proto,
		})
	}

	volumes := make([]docker.VolumeMount, 0, len(s.Volumes))
	for _, v := range s.Volumes {
		mode := v.Mode
		if mode == "" {
			mode = "rw"
		}
		volumes = append(volumes, docker.VolumeMount{
			Source: v.Source,
			Target: v.Target,
			Mode:   mode,
		})
	}

	env := make([]string, 0, len(s.Env))
	for k, v := range s.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	return docker.ContainerSpec{
		Name:    ContainerName(serviceID),
		Image:   s.Image,
		Cmd:     s.Command,
		Ports:   ports,
		Volumes: volumes,
		Env:     env,
		Restart: s.Restart,
	}
}
