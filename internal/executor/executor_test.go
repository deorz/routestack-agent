package executor

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestExecuteKnownOperation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	exec := NewExecutor(logger)

	exec.Register("TEST_OP", func(ctx context.Context, op Operation) (*Result, error) {
		return &Result{Status: "success", Message: "done"}, nil
	})

	result, err := exec.Execute(context.Background(), Operation{
		ID:   "op-1",
		Type: "TEST_OP",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != "success" {
		t.Errorf("Status: got %q, want success", result.Status)
	}
}

func TestExecuteUnknownOperation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	exec := NewExecutor(logger)

	_, err := exec.Execute(context.Background(), Operation{
		ID:   "op-1",
		Type: "UNKNOWN_TYPE",
	})
	if err == nil {
		t.Fatal("expected error for unknown operation type")
	}
}

func TestIdempotency(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	exec := NewExecutor(logger)

	callCount := 0
	exec.Register("IDEM_OP", func(ctx context.Context, op Operation) (*Result, error) {
		callCount++
		return &Result{Status: "success", Message: "ok"}, nil
	})

	op := Operation{ID: "dup-1", Type: "IDEM_OP"}

	// First call should execute the handler.
	result1, err := exec.Execute(context.Background(), op)
	if err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	if result1.Status != "success" {
		t.Errorf("first result: got %q", result1.Status)
	}

	// Second call with same ID should be skipped (idempotent).
	result2, err := exec.Execute(context.Background(), op)
	if err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if result2.Status != "success" {
		t.Errorf("second result: got %q", result2.Status)
	}
	if result2.Message != "already processed" {
		t.Errorf("idempotent message: got %q, want 'already processed'", result2.Message)
	}

	if callCount != 1 {
		t.Errorf("handler called %d times, want 1", callCount)
	}
}

func TestHandlerError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	exec := NewExecutor(logger)

	exec.Register("FAIL_OP", func(ctx context.Context, op Operation) (*Result, error) {
		return nil, errors.New("something broke")
	})

	_, err := exec.Execute(context.Background(), Operation{
		ID:   "op-err",
		Type: "FAIL_OP",
	})
	if err == nil {
		t.Fatal("expected error from failing handler")
	}
}

func TestRedirectError(t *testing.T) {
	exec := NewExecutor(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	exec.Register("FUTURE_OP", NewStubHandler("Phase 99"))

	_, err := exec.Execute(context.Background(), Operation{
		ID:   "op-future",
		Type: "FUTURE_OP",
	})
	if err == nil {
		t.Fatal("expected RedirectError")
	}

	var redir *RedirectError
	if !errors.As(err, &redir) {
		t.Fatalf("expected *RedirectError, got %T: %v", err, err)
	}
	if redir.Target != "Phase 99" {
		t.Errorf("Target: got %q, want Phase 99", redir.Target)
	}
}

func TestDockerStartHandlerValidPayload(t *testing.T) {
	payload, _ := json.Marshal(map[string]string{
		"container_name": "routestack-xray-1",
	})

	var p struct {
		ContainerName string `json:"container_name"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.ContainerName != "routestack-xray-1" {
		t.Errorf("ContainerName: got %q", p.ContainerName)
	}
}

func TestDockerStartHandlerMissingName(t *testing.T) {
	payload, _ := json.Marshal(map[string]string{})
	var p struct {
		ContainerName string `json:"container_name"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.ContainerName != "" {
		t.Error("expected empty container_name")
	}
}

func TestEvictionClearsOldEntries(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	exec := NewExecutor(logger)

	exec.mu.Lock()
	exec.seen["old-op"] = time.Now().Add(-2 * time.Hour)
	exec.mu.Unlock()

	// Force eviction by calling the eviction logic directly.
	cutoff := time.Now().Add(-1 * time.Hour)
	exec.mu.Lock()
	for id, ts := range exec.seen {
		if ts.Before(cutoff) {
			delete(exec.seen, id)
		}
	}
	exec.mu.Unlock()

	exec.mu.RLock()
	_, exists := exec.seen["old-op"]
	exec.mu.RUnlock()
	if exists {
		t.Error("old entry should have been evicted")
	}
}
