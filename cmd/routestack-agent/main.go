package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"routestack-agent/internal/agent"
	"routestack-agent/internal/api"
	"routestack-agent/internal/docker"
	"routestack-agent/internal/executor"
	"routestack-agent/internal/filesystem"
	"routestack-agent/internal/installer"
	"routestack-agent/internal/state"
)

// Populated at build time via -ldflags.
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		cmdRun(os.Args[2:])
	case "enroll":
		cmdEnroll(os.Args[2:])
	case "version":
		fmt.Printf("routestack-agent %s (%s)\n", version, commit)
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage:
  routestack-agent run [--config <path>]
  routestack-agent enroll --token <token> --controller <url> [--config <path>]
  routestack-agent version
`)
}

// ── run ──────────────────────────────────────────────────────────────────────

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath(), "path to config.yaml")
	_ = fs.Parse(args)

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := agent.LoadConfig(*configPath)
	if err != nil {
		logger.Error("failed to load config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	st, err := state.LoadState(cfg.Paths.StateFile)
	if err != nil {
		logger.Error("failed to load state", slog.String("error", err.Error()), slog.String("path", cfg.Paths.StateFile))
		logger.Error("state file may be corrupted — remove it and re-enroll")
		os.Exit(1)
	}

	if !st.IsEnrolled() {
		logger.Error("agent not enrolled", slog.String("hint", "run 'routestack-agent enroll --token <token> --controller <url>'"))
		os.Exit(1)
	}

	// Build the mTLS API client from stored cert paths.
	apiClient, err := api.NewClient(
		cfg.Controller.URL,
		st.CertPath,
		st.KeyPath,
		cfg.Controller.CACertPath,
		time.Duration(cfg.Controller.ConnectTimeoutSec)*time.Second,
		time.Duration(cfg.Controller.RequestTimeoutSec)*time.Second,
	)
	if err != nil {
		logger.Error("failed to create API client", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Adapt state.AgentState → agent.StateStore interface.
	stateStore := &stateAdapter{st}

	// Wire executor with Docker-based operation handlers.
	exec := executor.NewExecutor(logger)
	dockerClient, dockerErr := docker.NewClient()
	if dockerErr != nil {
		logger.Warn("Docker not available — container operations will fail",
			slog.String("error", dockerErr.Error()))
	}

	// Register handlers for implemented operation types.
	if dockerClient != nil {
		exec.Register(executor.OpStartService, executor.NewDockerStartHandler(dockerClient))
		exec.Register(executor.OpStopService, executor.NewDockerStopHandler(dockerClient))
		exec.Register(executor.OpRestartService, executor.NewDockerRestartHandler(dockerClient))

		inst := installer.NewInstaller(dockerClient)
		exec.Register(executor.OpInstallComponent, executor.NewInstallComponentHandler(inst))
	}

	// Stubs for operation types scheduled in later phases.
	exec.Register(executor.OpApplyServiceRevision, executor.NewStubHandler("Phase 5"))
	exec.Register(executor.OpApplyFirewallRevision, executor.NewStubHandler("Phase 12"))
	exec.Register(executor.OpCreateTunnel, executor.NewStubHandler("Phase 6"))
	exec.Register(executor.OpRemoveTunnel, executor.NewStubHandler("Phase 6"))
	exec.Register(executor.OpCollectStatus, executor.NewStubHandler("Phase 5"))
	exec.Register(executor.OpCollectLogs, executor.NewStubHandler("Phase 5"))
	exec.Register(executor.OpRunHealthCheck, executor.NewStubHandler("Phase 13"))
	exec.Register(executor.OpManageCertificate, executor.NewStubHandler("Phase 14"))

	a := agent.NewAgent(cfg, stateStore, apiClient, exec, version, logger)

	if err := a.Run(context.Background()); err != nil {
		logger.Error("agent failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

// ── enroll ───────────────────────────────────────────────────────────────────

func cmdEnroll(args []string) {
	fs := flag.NewFlagSet("enroll", flag.ExitOnError)
	token := fs.String("token", "", "enrollment token from controller")
	controllerURL := fs.String("controller", "", "controller URL (e.g. https://panel.example.com)")
	configPath := fs.String("config", defaultConfigPath(), "path to config.yaml")
	_ = fs.Parse(args)

	if *token == "" || *controllerURL == "" {
		fmt.Fprintln(os.Stderr, "error: --token and --controller are required")
		fs.Usage()
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Load config to get CA cert path.
	cfg, err := agent.LoadConfig(*configPath)
	if err != nil {
		logger.Error("failed to load config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	caCertPath := cfg.Controller.CACertPath
	if caCertPath == "" {
		caCertPath = filesystem.AgentConfigDir + "/ca.pem"
	}

	// Check CA cert exists.
	if _, err := os.Stat(caCertPath); err != nil {
		logger.Error("CA certificate not found", slog.String("path", caCertPath),
			slog.String("hint", "place the controller CA certificate at this path"))
		os.Exit(1)
	}

	logger.Info("enrolling with controller", slog.String("url", *controllerURL))

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	resp, err := api.Enroll(ctx, *controllerURL, *token, caCertPath)
	if err != nil {
		cancel()
		logger.Error("enrollment failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Update persistent state with enrollment data.
	st, err := state.LoadState(cfg.Paths.StateFile)
	if err != nil {
		logger.Error("failed to load state after enrollment", slog.String("error", err.Error()))
		os.Exit(1)
	}

	st.NodeID = resp.NodeID
	st.CertPath = filesystem.DefaultCertPath
	st.KeyPath = filesystem.DefaultKeyPath

	if err := st.Save(); err != nil {
		logger.Error("failed to save state", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("enrollment complete",
		slog.String("node_id", resp.NodeID),
		slog.String("cert_path", st.CertPath),
		slog.String("key_path", st.KeyPath),
	)

	fmt.Printf("Enrolled as node %s\n", resp.NodeID)
}

// ── helpers ──────────────────────────────────────────────────────────────────

func defaultConfigPath() string {
	if p := os.Getenv("ROUTESTACK_CONFIG"); p != "" {
		return p
	}
	return filesystem.AgentConfigDir + "/config.yaml"
}

// stateAdapter wraps *state.AgentState to implement agent.StateStore.
// Defined here rather than in the state package to keep the state package
// free of agent-layer interface knowledge.
type stateAdapter struct {
	s *state.AgentState
}

func (a *stateAdapter) IsEnrolled() bool { return a.s.IsEnrolled() }
func (a *stateAdapter) NodeID() string   { return a.s.NodeID }
func (a *stateAdapter) Save() error      { return a.s.Save() }
