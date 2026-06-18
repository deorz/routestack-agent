// Package docker provides a minimal Docker Engine API client over
// the Unix socket (/var/run/docker.sock). It covers only the container
// lifecycle operations the agent needs: pull, create, start, stop,
// remove, inspect, list, and logs.
package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Client is a minimal Docker Engine API client over a Unix socket.
type Client struct {
	httpClient *http.Client
	apiVersion string
}

// NewClient connects to the Docker daemon at the default Unix socket path.
// Use DOCKER_HOST env override if set (for testing).
func NewClient() (*Client, error) {
	socketPath := "/var/run/docker.sock"

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
		MaxIdleConns:    5,
		IdleConnTimeout: 60 * time.Second,
	}

	return &Client{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   120 * time.Second,
		},
		apiVersion: "v1.45",
	}, nil
}

// ── Types ────────────────────────────────────────────────────────────────────

// ContainerSpec describes a container to be created.
type ContainerSpec struct {
	Name    string
	Image   string
	Ports   []PortMapping
	Volumes []VolumeMount
	Env     []string
	Restart string // "always", "unless-stopped", "no"
	Cmd     []string
}

// PortMapping maps a host port to a container port.
type PortMapping struct {
	HostPort      string
	ContainerPort string
	Protocol      string // "tcp" or "udp"
}

// VolumeMount maps a host path to a container path.
type VolumeMount struct {
	Source string // host path
	Target string // container path
	Mode   string // "ro" or "rw"
}

// ContainerInfo holds a summary of a container (from GET /containers/json).
type ContainerInfo struct {
	ID     string `json:"Id"`
	Names  []string
	Image  string
	State  string
	Status string
	Ports  []ContainerPort `json:"Ports"`
}

// ContainerPort describes an exposed port on a container.
type ContainerPort struct {
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort"`
	Type        string `json:"Type"`
}

// ContainerState holds a detailed view of a container (from GET /containers/{id}/json).
type ContainerState struct {
	ID      string `json:"Id"`
	Created string `json:"Created"`
	State   struct {
		Status     string `json:"Status"`
		Running    bool   `json:"Running"`
		ExitCode   int    `json:"ExitCode"`
		StartedAt  string `json:"StartedAt"`
		FinishedAt string `json:"FinishedAt"`
		Health     *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	Name string `json:"Name"`
}

// ── Container lifecycle ──────────────────────────────────────────────────────

// Pull downloads an image from the registry. It follows the pull progress
// stream until complete or an error occurs.
func (c *Client) Pull(ctx context.Context, image string) error {
	q := fmt.Sprintf("/%s/images/create?fromImage=%s", c.apiVersion, image)
	resp, err := c.do(ctx, http.MethodPost, q, nil)
	if err != nil {
		return fmt.Errorf("docker pull %s: %w", image, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("docker pull %s: http %d: %s", image, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// Consume and discard the pull stream. Each line is a JSON object.
	// We just need to know it completed without error.
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var msg struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &msg); err == nil && msg.Error != "" {
			return fmt.Errorf("docker pull %s: %s", image, msg.Error)
		}
	}
	return nil
}

// ImageInfo holds metadata returned by image inspection.
type ImageInfo struct {
	ID           string   `json:"Id"`
	RepoTags     []string `json:"RepoTags"`
	RepoDigests  []string `json:"RepoDigests"`
	Size         int64    `json:"Size"`
	Os           string   `json:"Os"`
	Architecture string   `json:"Architecture"`
}

// InspectImage returns metadata for an image by reference.
func (c *Client) InspectImage(ctx context.Context, image string) (*ImageInfo, error) {
	q := fmt.Sprintf("/%s/images/%s/json", c.apiVersion, image)
	resp, err := c.do(ctx, http.MethodGet, q, nil)
	if err != nil {
		return nil, fmt.Errorf("docker inspect image %s: %w", image, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("docker inspect image %s: http %d: %s", image, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var info ImageInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("docker inspect image %s: decode: %w", image, err)
	}
	return &info, nil
}

// Create creates a container from the given spec. Returns the container ID.
func (c *Client) Create(ctx context.Context, spec ContainerSpec) (string, error) {
	body := c.buildCreateRequest(spec)
	q := fmt.Sprintf("/%s/containers/create?name=%s", c.apiVersion, spec.Name)

	resp, err := c.doJSON(ctx, http.MethodPost, q, body)
	if err != nil {
		return "", fmt.Errorf("docker create %s: %w", spec.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("docker create %s: http %d: %s", spec.Name, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var result struct {
		ID       string   `json:"Id"`
		Warnings []string `json:"Warnings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("docker create %s: decode: %w", spec.Name, err)
	}

	return result.ID, nil
}

// Start starts a container by ID or name.
func (c *Client) Start(ctx context.Context, containerID string) error {
	q := fmt.Sprintf("/%s/containers/%s/start", c.apiVersion, containerID)
	resp, err := c.do(ctx, http.MethodPost, q, nil)
	if err != nil {
		return fmt.Errorf("docker start %s: %w", containerID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("docker start %s: http %d: %s", containerID, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// Stop stops a container by ID or name, with an optional timeout in seconds.
// If timeout is 0, Docker uses its default (10s).
func (c *Client) Stop(ctx context.Context, containerID string, timeoutSec int) error {
	q := fmt.Sprintf("/%s/containers/%s/stop", c.apiVersion, containerID)
	if timeoutSec > 0 {
		q += fmt.Sprintf("?t=%d", timeoutSec)
	}
	resp, err := c.do(ctx, http.MethodPost, q, nil)
	if err != nil {
		return fmt.Errorf("docker stop %s: %w", containerID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("docker stop %s: http %d: %s", containerID, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// Remove deletes a container by ID or name. If force is true, it removes
// even if running.
func (c *Client) Remove(ctx context.Context, containerID string, force bool) error {
	q := fmt.Sprintf("/%s/containers/%s", c.apiVersion, containerID)
	if force {
		q += "?force=true"
	}
	resp, err := c.do(ctx, http.MethodDelete, q, nil)
	if err != nil {
		return fmt.Errorf("docker remove %s: %w", containerID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("docker remove %s: http %d: %s", containerID, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// Inspect returns detailed state for a container.
func (c *Client) Inspect(ctx context.Context, containerID string) (*ContainerState, error) {
	q := fmt.Sprintf("/%s/containers/%s/json", c.apiVersion, containerID)

	resp, err := c.do(ctx, http.MethodGet, q, nil)
	if err != nil {
		return nil, fmt.Errorf("docker inspect %s: %w", containerID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("docker inspect %s: http %d: %s", containerID, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var state ContainerState
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return nil, fmt.Errorf("docker inspect %s: decode: %w", containerID, err)
	}
	return &state, nil
}

// KnownContainerNames is the set of container names the agent can adopt
// even if they were not originally created by the agent. This covers
// containers deployed by external apps such as AmneziaVPN.
//
// Names mirror the Amnezia-Web-Panel convention:
//
//	https://github.com/PRVTPRO/Amnezia-Web-Panel
var KnownContainerNames = []string{
	"amnezia-awg2",
	"amnezia-xray",
	"telemt",
}

// FindContainer looks up a container by exact name, then by known
// adoptable names, then by the routestack label. It returns the first
// match or an empty string if no container is found.
func (c *Client) FindContainer(ctx context.Context, name string) (string, error) {
	// 1. Exact name match.
	if state, err := c.Inspect(ctx, name); err == nil && state != nil {
		return state.ID, nil
	}

	// 2. Try known adoptable names.
	for _, candidate := range KnownContainerNames {
		if candidate == name {
			continue // already tried above
		}
		if state, err := c.Inspect(ctx, candidate); err == nil && state != nil {
			return state.ID, nil
		}
	}

	// 3. List routestack-labeled containers and match by name prefix.
	containers, err := c.List(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("find container %s: %w", name, err)
	}
	for _, container := range containers {
		for _, n := range container.Names {
			// Docker names start with '/'.
			clean := strings.TrimPrefix(n, "/")
			if clean == name {
				return container.ID, nil
			}
			for _, candidate := range KnownContainerNames {
				if clean == candidate {
					return container.ID, nil
				}
			}
		}
	}

	return "", nil
}

// Adopt adds the managed-by=routestack label to an existing container
// so the agent can track it going forward.
func (c *Client) Adopt(ctx context.Context, containerID string) error {
	q := fmt.Sprintf("/%s/containers/%s/update", c.apiVersion, containerID)
	body := map[string]any{
		"Labels": map[string]string{
			"managed-by": "routestack",
		},
	}

	resp, err := c.doJSON(ctx, http.MethodPost, q, body)
	if err != nil {
		return fmt.Errorf("docker adopt %s: %w", containerID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("docker adopt %s: http %d: %s", containerID, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return nil
}

// List returns containers matching the given label filter.
// If no labels are provided, it lists routestack-managed containers.
func (c *Client) List(ctx context.Context, labels map[string]string) ([]ContainerInfo, error) {
	q := fmt.Sprintf("/%s/containers/json?all=true", c.apiVersion)
	for k, v := range labels {
		q += fmt.Sprintf("&filters={\"label\":{%q:true}}", k+"="+v)
	}
	// If no labels, just list all.
	if len(labels) == 0 {
		// List only routestack-managed containers.
		q += "&filters={\"label\":{\"managed-by=routestack\":true}}"
	}

	resp, err := c.do(ctx, http.MethodGet, q, nil)
	if err != nil {
		return nil, fmt.Errorf("docker list: %w", err)
	}
	defer resp.Body.Close()

	var containers []ContainerInfo
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, fmt.Errorf("docker list: decode: %w", err)
	}
	return containers, nil
}

// Logs retrieves the last N lines of container logs.
func (c *Client) Logs(ctx context.Context, containerID string, tail int) (string, error) {
	q := fmt.Sprintf("/%s/containers/%s/logs?stdout=true&stderr=true&tail=%d", c.apiVersion, containerID, tail)

	resp, err := c.do(ctx, http.MethodGet, q, nil)
	if err != nil {
		return "", fmt.Errorf("docker logs %s: %w", containerID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("docker logs %s: http %d: %s", containerID, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("docker logs %s: read: %w", containerID, err)
	}
	return string(data), nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// do performs an HTTP request and returns the response.
func (c *Client) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	url := "http://unix" + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.httpClient.Do(req)
}

// doJSON marshals body as JSON and sends the request.
func (c *Client) doJSON(ctx context.Context, method, path string, body any) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}
	return c.do(ctx, method, path, bytes.NewReader(data))
}

// buildCreateRequest constructs the Docker API create container request body.
func (c *Client) buildCreateRequest(spec ContainerSpec) map[string]any {
	req := map[string]any{
		"Image": spec.Image,
	}

	if len(spec.Cmd) > 0 {
		req["Cmd"] = spec.Cmd
	}

	// Port bindings.
	if len(spec.Ports) > 0 {
		portBindings := map[string]any{}
		exposedPorts := map[string]any{}
		for _, p := range spec.Ports {
			containerPort := p.ContainerPort + "/" + p.Protocol
			exposedPorts[containerPort] = map[string]any{}
			portBindings[containerPort] = []map[string]any{
				{"HostPort": p.HostPort},
			}
		}
		req["ExposedPorts"] = exposedPorts
		req["HostConfig"] = map[string]any{
			"PortBindings": portBindings,
		}
	}

	// Volume mounts.
	if len(spec.Volumes) > 0 {
		binds := make([]string, 0, len(spec.Volumes))
		for _, v := range spec.Volumes {
			mode := v.Mode
			if mode == "" {
				mode = "rw"
			}
			binds = append(binds, v.Source+":"+v.Target+":"+mode)
		}
		hostCfg, _ := req["HostConfig"].(map[string]any)
		if hostCfg == nil {
			hostCfg = map[string]any{}
			req["HostConfig"] = hostCfg
		}
		hostCfg["Binds"] = binds
	}

	// Environment variables.
	if len(spec.Env) > 0 {
		envList := make([]string, 0, len(spec.Env))
		envList = append(envList, spec.Env...)
		req["Env"] = envList
	}

	// Restart policy.
	if spec.Restart != "" && spec.Restart != "no" {
		hostCfg, _ := req["HostConfig"].(map[string]any)
		if hostCfg == nil {
			hostCfg = map[string]any{}
			req["HostConfig"] = hostCfg
		}
		hostCfg["RestartPolicy"] = map[string]any{
			"Name": spec.Restart,
		}
	}

	// Labels for routestack namespace.
	req["Labels"] = map[string]string{
		"managed-by": "routestack",
	}

	return req
}
