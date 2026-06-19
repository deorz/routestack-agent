// Package firewall applies host firewall rulesets for the node.
//
// The control plane sends a revision id and a list of iptables rules.
// The manager applies them atomically by flushing the routestack chain,
// recreating it, and linking it into the DOCKER-USER chain.
package firewall

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Rule is a single iptables rule in arguments form (without the leading
// "iptables" binary). Example: []string{"-A", "ROUTESTACK-FWD", "-p", "tcp", "--dport", "443", "-j", "ACCEPT"}
type Rule struct {
	Args []string `json:"args"`
}

// Ruleset describes a firewall revision.
type Ruleset struct {
	Revision int64  `json:"revision"`
	Rules    []Rule `json:"rules"`
}

// CommandRunner abstracts os/exec for testing.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) error
	Output(ctx context.Context, name string, args ...string) (string, error)
}

// execRunner is the default runner backed by os/exec.
type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}

func (execRunner) Output(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}

// Manager applies firewall rulesets.
type Manager struct {
	runner CommandRunner
}

// NewManager creates a firewall manager.
func NewManager() *Manager {
	return &Manager{runner: execRunner{}}
}

// NewManagerWithRunner creates a firewall manager with a custom command runner.
func NewManagerWithRunner(r CommandRunner) *Manager {
	return &Manager{runner: r}
}

const chainName = "ROUTESTACK-FWD"

// ApplyRevision atomically replaces the routestack firewall chain with the
// given ruleset. It is safe to call multiple times; later calls override
// earlier revisions.
func (m *Manager) ApplyRevision(ctx context.Context, rs Ruleset) error {
	if rs.Revision < 0 {
		return fmt.Errorf("revision must be non-negative")
	}

	// Ensure the routestack chain exists.
	if err := m.ensureChain(ctx); err != nil {
		return err
	}

	// Flush existing rules from the chain.
	if err := m.runner.Run(ctx, "iptables", "-F", chainName); err != nil {
		return fmt.Errorf("flush %s: %w", chainName, err)
	}

	// Add new rules.
	for i, rule := range rs.Rules {
		if len(rule.Args) == 0 {
			continue
		}
		args := append([]string{"-A", chainName}, rule.Args...)
		if err := m.runner.Run(ctx, "iptables", args...); err != nil {
			return fmt.Errorf("apply rule %d (%v): %w", i, rule.Args, err)
		}
	}

	return nil
}

// ensureChain creates the routestack chain and links it to DOCKER-USER if needed.
func (m *Manager) ensureChain(ctx context.Context) error {
	// Create chain if it does not exist (iptables -N fails if it exists).
	_ = m.runner.Run(ctx, "iptables", "-N", chainName)

	// Link to DOCKER-USER if not already linked.
	out, err := m.runner.Output(ctx, "iptables", "-L", "DOCKER-USER", "--line-numbers", "-n")
	if err != nil {
		// DOCKER-USER may not exist if Docker is not running. Create it.
		_ = m.runner.Run(ctx, "iptables", "-N", "DOCKER-USER")
	} else if strings.Contains(out, chainName) {
		return nil
	}

	return m.runner.Run(ctx, "iptables", "-I", "DOCKER-USER", "1", "-j", chainName)
}

// RevisionFromString parses a revision id from string.
func RevisionFromString(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
