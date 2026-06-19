package firewall

import (
	"context"
	"fmt"
	"testing"
)

type mockRunner struct {
	commands [][]string
	outputs  map[string]string
	fail     map[string]bool
}

func (m *mockRunner) Run(_ context.Context, name string, args ...string) error {
	full := append([]string{name}, args...)
	m.commands = append(m.commands, full)
	key := cmdKey(name, args)
	if m.fail[key] {
		return fmt.Errorf("command failed: %s %v", name, args)
	}
	return nil
}

func (m *mockRunner) Output(_ context.Context, name string, args ...string) (string, error) {
	full := append([]string{name}, args...)
	m.commands = append(m.commands, full)
	key := cmdKey(name, args)
	if out, ok := m.outputs[key]; ok {
		return out, nil
	}
	return "", fmt.Errorf("output not set: %s %v", name, args)
}

func cmdKey(name string, args []string) string {
	return fmt.Sprintf("%s %v", name, args)
}

func TestApplyRevision(t *testing.T) {
	runner := &mockRunner{
		outputs: map[string]string{
			cmdKey("iptables", []string{"-L", "DOCKER-USER", "--line-numbers", "-n"}): "Chain DOCKER-USER (1 references)\nnum  target\n",
		},
	}
	mgr := NewManagerWithRunner(runner)

	rs := Ruleset{
		Revision: 5,
		Rules: []Rule{
			{Args: []string{"-p", "tcp", "--dport", "443", "-j", "ACCEPT"}},
		},
	}

	if err := mgr.ApplyRevision(context.Background(), rs); err != nil {
		t.Fatalf("ApplyRevision failed: %v", err)
	}

	// Expected commands:
	// 1. create chain (ignored if exists)
	// 2. list DOCKER-USER
	// 3. insert jump to chain
	// 4. flush chain
	// 5. append rule
	if len(runner.commands) < 4 {
		t.Fatalf("expected at least 4 commands, got %v", runner.commands)
	}
}

func TestApplyRevisionNoRules(t *testing.T) {
	runner := &mockRunner{
		outputs: map[string]string{
			cmdKey("iptables", []string{"-L", "DOCKER-USER", "--line-numbers", "-n"}): "Chain DOCKER-USER (1 references)\n",
		},
	}
	mgr := NewManagerWithRunner(runner)

	rs := Ruleset{Revision: 1, Rules: nil}
	if err := mgr.ApplyRevision(context.Background(), rs); err != nil {
		t.Fatalf("ApplyRevision failed: %v", err)
	}

	var flushed bool
	for _, cmd := range runner.commands {
		if len(cmd) >= 3 && cmd[0] == "iptables" && cmd[1] == "-F" && cmd[2] == chainName {
			flushed = true
		}
	}
	if !flushed {
		t.Error("chain was not flushed")
	}
}

func TestApplyRevisionNegativeRevision(t *testing.T) {
	mgr := NewManagerWithRunner(&mockRunner{})
	rs := Ruleset{Revision: -1}
	if err := mgr.ApplyRevision(context.Background(), rs); err == nil {
		t.Fatal("expected error for negative revision")
	}
}

func TestEnsureChainAlreadyLinked(t *testing.T) {
	runner := &mockRunner{
		outputs: map[string]string{
			cmdKey("iptables", []string{"-L", "DOCKER-USER", "--line-numbers", "-n"}): "Chain DOCKER-USER (1 references)\nnum  target\n1    ROUTESTACK-FWD\n",
		},
	}
	mgr := NewManagerWithRunner(runner)

	rs := Ruleset{Revision: 2, Rules: []Rule{{Args: []string{"-j", "RETURN"}}}}
	if err := mgr.ApplyRevision(context.Background(), rs); err != nil {
		t.Fatalf("ApplyRevision failed: %v", err)
	}

	// Should not try to insert jump when already linked.
	for _, cmd := range runner.commands {
		if len(cmd) >= 3 && cmd[0] == "iptables" && cmd[1] == "-I" {
			t.Errorf("unexpected insert command: %v", cmd)
		}
	}
}

func TestRevisionFromString(t *testing.T) {
	rev, err := RevisionFromString("42")
	if err != nil || rev != 42 {
		t.Errorf("RevisionFromString(42) = %d, %v", rev, err)
	}
	if _, err := RevisionFromString("abc"); err == nil {
		t.Error("expected error for non-numeric revision")
	}
}
