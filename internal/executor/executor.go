// Package executor implements operation dispatch, validation, and idempotency.
//
// Operation handlers are registered per operation type. The executor validates
// that an operation type is known, checks that the operation ID has not been
// recently processed (idempotency), and dispatches to the registered handler
// with a timeout.
package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Operation type constants shared with the Python control plane contract.
const (
	OpInstallComponent      = "INSTALL_COMPONENT"
	OpApplyServiceRevision  = "APPLY_SERVICE_REVISION"
	OpStartService          = "START_SERVICE"
	OpStopService           = "STOP_SERVICE"
	OpRestartService        = "RESTART_SERVICE"
	OpApplyFirewallRevision = "APPLY_FIREWALL_REVISION"
	OpCreateTunnel          = "CREATE_TUNNEL"
	OpRemoveTunnel          = "REMOVE_TUNNEL"
	OpCollectStatus         = "COLLECT_STATUS"
	OpCollectLogs           = "COLLECT_LOGS"
	OpRunHealthCheck        = "RUN_HEALTH_CHECK"
	OpManageCertificate     = "MANAGE_CERTIFICATE"
)

// Operation is a single task assigned to the agent by the controller.
type Operation struct {
	ID   string          `json:"id"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// Result holds the outcome of executing an operation.
type Result struct {
	Status  string `json:"status"` // "success", "degraded", "failed"
	Message string `json:"message"`
}

// Handler is a function that executes a specific operation type.
// It receives the full operation for access to ID and payload.
type Handler func(ctx context.Context, op Operation) (*Result, error)

// Executor dispatches operations to registered handlers with validation
// and idempotency guarantees.
type Executor struct {
	mu       sync.RWMutex
	handlers map[string]Handler
	seen     map[string]time.Time // recently processed operation IDs
	logger   *slog.Logger

	// Default timeout for handler execution.
	defaultTimeout time.Duration
}

// NewExecutor creates an executor with no registered handlers.
func NewExecutor(logger *slog.Logger) *Executor {
	e := &Executor{
		handlers:       make(map[string]Handler),
		seen:           make(map[string]time.Time),
		logger:         logger,
		defaultTimeout: 5 * time.Minute,
	}
	// Start background eviction of old seen entries.
	go e.evictLoop()
	return e
}

// Register associates a handler with an operation type.
// Panics if the type is already registered (startup misconfiguration).
func (e *Executor) Register(opType string, handler Handler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.handlers[opType]; ok {
		panic(fmt.Sprintf("executor: handler for %q already registered", opType))
	}
	e.handlers[opType] = handler
}

// Execute validates and dispatches an operation.
//
// Validation steps:
//  1. Operation type must be registered.
//  2. Operation ID must not have been processed (idempotency — 1-hour window).
//  3. Handler is invoked with a timeout context.
//
// On success, the operation ID is recorded to prevent replays.
func (e *Executor) Execute(ctx context.Context, op Operation) (*Result, error) {
	// 1. Check handler exists.
	e.mu.RLock()
	handler, ok := e.handlers[op.Type]
	e.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("executor: unknown operation type %q", op.Type)
	}

	// 2. Idempotency check.
	e.mu.Lock()
	if _, seen := e.seen[op.ID]; seen {
		e.mu.Unlock()
		e.logger.Warn("duplicate operation skipped", slog.String("op_id", op.ID), slog.String("op_type", op.Type))
		return &Result{Status: "success", Message: "already processed"}, nil
	}
	// Mark seen before execution to prevent concurrent replays.
	e.seen[op.ID] = time.Now()
	e.mu.Unlock()

	// 3. Execute with timeout.
	execCtx, cancel := context.WithTimeout(ctx, e.defaultTimeout)
	defer cancel()

	start := time.Now()
	result, err := handler(execCtx, op)
	elapsed := time.Since(start)

	if err != nil {
		e.logger.Error("operation failed",
			slog.String("op_id", op.ID),
			slog.String("op_type", op.Type),
			slog.Duration("elapsed", elapsed),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("executor: %s %s: %w", op.Type, op.ID, err)
	}

	e.logger.Info("operation completed",
		slog.String("op_id", op.ID),
		slog.String("op_type", op.Type),
		slog.String("status", result.Status),
		slog.Duration("elapsed", elapsed),
	)

	return result, nil
}

// evictLoop periodically cleans up seen entries older than 1 hour.
func (e *Executor) evictLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		e.mu.Lock()
		cutoff := time.Now().Add(-1 * time.Hour)
		for id, t := range e.seen {
			if t.Before(cutoff) {
				delete(e.seen, id)
			}
		}
		e.mu.Unlock()
	}
}

// RedirectError is sentinel for "not yet implemented" operation types.
type RedirectError struct {
	OpType string
	Target string // which phase will implement it
}

func (e *RedirectError) Error() string {
	return fmt.Sprintf("%s: not yet implemented — scheduled for %s", e.OpType, e.Target)
}
