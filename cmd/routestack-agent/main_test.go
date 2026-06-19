package main

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"routestack-agent/internal/docker"
	"routestack-agent/internal/executor"
)

func TestRegisterDockerHandlersIncludesCollectStatus(t *testing.T) {
	dockerClient, err := docker.NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	exec := executor.NewExecutor(slog.New(slog.NewTextHandler(io.Discard, nil)))
	registerDockerHandlers(exec, dockerClient)

	_, err = exec.Execute(context.Background(), executor.Operation{
		ID:   "op-status-registration",
		Type: executor.OpCollectStatus,
		Data: []byte(`{}`),
	})
	if err == nil {
		t.Fatal("expected validation error from COLLECT_STATUS handler")
	}
	if strings.Contains(err.Error(), "unknown operation") {
		t.Fatalf("COLLECT_STATUS handler was not registered: %v", err)
	}
	if !strings.Contains(err.Error(), "service_id is required") {
		t.Fatalf("unexpected COLLECT_STATUS error: %v", err)
	}
}
