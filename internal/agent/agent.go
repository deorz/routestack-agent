package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"routestack-agent/internal/api"
	"routestack-agent/internal/executor"
)

// APIClient is the interface the Agent uses to communicate with the
// control plane. Injecting this as an interface allows testing Agent
// without a real HTTP client.
type APIClient interface {
	Heartbeat(ctx context.Context, req api.HeartbeatRequest) (*api.HeartbeatResponse, error)
	ClaimOperation(ctx context.Context, req api.ClaimRequest) (*api.ClaimResponse, error)
	CompleteOperation(ctx context.Context, opID string, req api.CompleteRequest) error
}

// StateStore is the interface the Agent uses for persistent state.
type StateStore interface {
	IsEnrolled() bool
	NodeID() string
	Save() error
}

// Agent is the core agent runtime. It owns the main loop, heartbeat ticker,
// and operation poller goroutines.
type Agent struct {
	config   *Config
	state    StateStore
	api      APIClient
	executor *executor.Executor
	logger   *slog.Logger
	version  string
}

// NewAgent creates an Agent with its wired dependencies.
func NewAgent(cfg *Config, st StateStore, apiClient APIClient, exec *executor.Executor, version string, logger *slog.Logger) *Agent {
	return &Agent{
		config:   cfg,
		state:    st,
		api:      apiClient,
		executor: exec,
		logger:   logger,
		version:  version,
	}
}

// Run starts the agent's main loop. It blocks until a shutdown signal
// (SIGTERM, SIGINT) or the context is cancelled, then performs
// graceful shutdown with a 30-second deadline.
func (a *Agent) Run(ctx context.Context) error {
	if !a.state.IsEnrolled() {
		return fmt.Errorf("agent is not enrolled: run 'routestack-agent enroll' first")
	}

	a.logger.Info("agent starting",
		slog.String("version", a.version),
		slog.String("node_id", a.state.NodeID()),
		slog.String("controller", a.config.Controller.URL),
	)

	// Detect and log capabilities.
	caps := api.DetectCapabilities()
	a.logger.Info("detected capabilities", slog.Any("capabilities", caps))

	// Signal-aware context.
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	var wg sync.WaitGroup

	// Heartbeat goroutine.
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.heartbeatLoop(ctx, caps)
	}()

	// Operation poll goroutine (stub — implemented in Phase 2).
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.operationPollLoop(ctx)
	}()

	// Block until shutdown.
	<-ctx.Done()
	a.logger.Info("agent shutting down")

	// Graceful shutdown with deadline.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		a.logger.Info("agent stopped gracefully")
	case <-time.After(30 * time.Second):
		a.logger.Warn("graceful shutdown timed out after 30s")
	}

	return nil
}

// heartbeatLoop sends periodic heartbeats to the controller.
func (a *Agent) heartbeatLoop(ctx context.Context, caps []string) {
	interval := time.Duration(a.config.Controller.HeartbeatIntervalSec) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Send first heartbeat immediately.
	a.sendHeartbeat(ctx, caps)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.sendHeartbeat(ctx, caps)
		}
	}
}

func (a *Agent) sendHeartbeat(ctx context.Context, caps []string) {
	osName, arch, kernel := api.CollectSystemInfo()
	metrics := api.CollectSystemMetrics()

	req := api.HeartbeatRequest{
		NodeID:        a.state.NodeID(),
		AgentVersion:  a.version,
		OS:            osName,
		Architecture:  arch,
		KernelVersion: kernel,
		Capabilities:  caps,
		System:        metrics,
		// Services field filled in Phase 5+ when service management is implemented.
	}

	_, err := a.api.Heartbeat(ctx, req)
	if err != nil {
		a.logger.Warn("heartbeat failed", slog.String("error", err.Error()))
		return
	}

	a.logger.Debug("heartbeat sent",
		slog.String("node_id", a.state.NodeID()),
		slog.Float64("cpu_percent", metrics.CPUPercent),
		slog.Int64("memory_used_mb", metrics.MemoryUsedMB),
	)
}

// operationPollLoop long-polls the controller for pending operations,
// dispatches them to the executor, and reports completion.
func (a *Agent) operationPollLoop(ctx context.Context) {
	a.logger.Info("operation poller started")

	pollTimeout := time.Duration(a.config.Controller.OperationPollTimeoutSec) * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		pollCtx, cancel := context.WithTimeout(ctx, pollTimeout)
		resp, err := a.api.ClaimOperation(pollCtx, api.ClaimRequest{
			NodeID: a.state.NodeID(),
		})
		cancel()

		if err != nil {
			a.logger.Warn("operation claim failed", slog.String("error", err.Error()))
			// Back off briefly before retrying.
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		if resp.Operation == nil {
			continue // no work available, poll again immediately
		}

		op := executor.Operation{
			ID:   resp.Operation.ID,
			Type: resp.Operation.Type,
			Data: resp.Operation.Data,
		}

		a.logger.Info("claimed operation",
			slog.String("op_id", op.ID),
			slog.String("op_type", op.Type),
		)

		result, execErr := a.executor.Execute(ctx, op)

		// Report completion back to controller.
		status := "success"
		errMsg := ""
		if execErr != nil {
			status = "failed"
			errMsg = execErr.Error()
		} else if result != nil {
			status = result.Status
			errMsg = result.Message
		}

		reportCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		reportErr := a.api.CompleteOperation(reportCtx, op.ID, api.CompleteRequest{
			ID:     op.ID,
			Status: status,
			Result: errMsg,
		})
		cancel()

		if reportErr != nil {
			a.logger.Error("failed to report operation completion",
				slog.String("op_id", op.ID),
				slog.String("error", reportErr.Error()),
			)
		}
	}
}
