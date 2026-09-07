package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/recipes"
)

func TestPaidExitShowsExplicitRemoteChoicesAndCancel(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config.toml", "root", "home", ApplicationDependencies{})
	m.instanceID = 456
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	for _, label := range []string{"Stop instance", "Leave running", "Destroy instance", "Cancel"} {
		if !strings.Contains(m.View(), label) {
			t.Fatalf("missing %q: %s", label, m.View())
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.exitConfirm || m.quitting {
		t.Fatal("cancel did not preserve session")
	}
}

func TestPaidExitStopWaitsForConfirmationAndKeepsFailureRetryable(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config.toml", "root", "home", ApplicationDependencies{})
	m.instanceID = 456
	var attempts atomic.Int32
	release := make(chan struct{})
	m.recovery = setup.InstanceRecovery{InstanceID: 456, Stop: func(context.Context) error {
		if attempts.Add(1) == 1 {
			<-release
			return errors.New("provider unavailable")
		}
		return nil
	}}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.quitting || m.destruction == nil {
		t.Fatal("stop did not wait")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if m.exitConfirm {
		t.Fatal("interrupt opened an escape during verified shutdown")
	}
	close(release)
	deliverRecoveryCommand(t, m, cmd)
	if m.quitting || !m.exitConfirm || !strings.Contains(m.View(), "Stop unconfirmed") {
		t.Fatalf("failure not retryable: %s", m.View())
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	deliverRecoveryCommand(t, m, cmd)
	if !m.quitting || m.stoppedID != 456 || attempts.Load() != 2 {
		t.Fatal("confirmed retry did not finish")
	}
	if !strings.Contains(m.paidWarning(), "stop confirmed") {
		t.Fatal("stop outcome missing")
	}
}

func TestPaidExitDestructionNeedsSecondExplicitApproval(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config.toml", "root", "home", ApplicationDependencies{})
	m.instanceID = 456
	var calls atomic.Int32
	m.recovery = setup.InstanceRecovery{InstanceID: 456, Destroy: func(context.Context) error { calls.Add(1); return nil }}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if calls.Load() != 0 || !strings.Contains(m.View(), "Destroy instance #456?") {
		t.Fatal("missing second approval")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Cancel is default on second dialog.
	if calls.Load() != 0 || m.quitting {
		t.Fatal("default destroyed instance")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	deliverRecoveryCommand(t, m, cmd)
	if calls.Load() != 1 || !m.quitting || m.instanceID != 0 {
		t.Fatal("approved destruction did not finish")
	}
}

func TestPaidExitLeaveRunningHasNoRemoteMutation(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config.toml", "root", "home", ApplicationDependencies{Recovery: func(context.Context, int) (setup.InstanceRecovery, error) {
		t.Fatal("leave requested remote capability")
		return setup.InstanceRecovery{}, nil
	}})
	m.instanceID = 456
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.quitting || m.instanceID != 456 {
		t.Fatal("leave running lost identity")
	}
}

func TestPaidExitRejectsCapabilityForAnotherInstance(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config.toml", "root", "home", ApplicationDependencies{Recovery: func(context.Context, int) (setup.InstanceRecovery, error) {
		return setup.InstanceRecovery{InstanceID: 999, Stop: func(context.Context) error { t.Fatal("wrong instance stopped"); return nil }}, nil
	}})
	m.instanceID = 456
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.quitting || !strings.Contains(m.View(), "Cannot") {
		t.Fatal("mismatched capability accepted")
	}
}

func TestPaidExitUsesRefreshedCapabilityAfterJoiningSetup(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config.toml", "root", "home", ApplicationDependencies{})
	m.instanceID = 456
	var called atomic.Int32
	m.recovery = setup.InstanceRecovery{InstanceID: 456, Stop: func(context.Context) error { return errors.New("retired checkpoint") }}
	ctx, cancel := context.WithCancel(context.Background())
	s := &applicationSession{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	m.session = s
	go func() {
		<-ctx.Done()
		s.mu.Lock()
		s.recovery = setup.InstanceRecovery{InstanceID: 456, Stop: func(context.Context) error { called.Add(1); return nil }}
		s.mu.Unlock()
		close(s.done)
	}()
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	deliverRecoveryCommand(t, m, cmd)
	if !m.quitting || called.Load() != 1 {
		t.Fatalf("stale capability used: %s", m.View())
	}
}

func TestPaidExitFailureVisibleInSmallTerminal(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config.toml", "root", "home", ApplicationDependencies{})
	m.instanceID = 456
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.View(), "Cannot") {
		t.Fatalf("failure hidden: %s", m.View())
	}
}

func TestSavedExitRefreshesCheckpointAtExecution(t *testing.T) {
	path := startTestConfig(t)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Provider = config.Provider{Kind: "vast", InstanceID: 456}
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	var seen string
	cap, err := savedInstanceRecovery(path, 456, func(checkpoint string) setup.InstanceRecovery {
		seen = checkpoint
		return setup.InstanceRecovery{InstanceID: 456, Stop: func(context.Context) error { return nil }}
	})
	if err != nil {
		t.Fatal(err)
	}
	r, err := recipes.QwenSoloRTX5090()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(setup.Checkpoint{Version: 1, InstanceID: 456, Recipe: r, IdentityFile: "key", KnownHostsDir: "hosts", Phase: "created"})
	if err := os.WriteFile(setup.CheckpointPath(path), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := cap.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if seen != setup.CheckpointPath(path) {
		t.Fatal("new checkpoint did not reach locked recovery constructor")
	}
}

func TestPaidExitEnterDoesNotSilentlyLeaveInstanceRunning(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config.toml", "root", "home", ApplicationDependencies{})
	m.instanceID = 456
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.quitting || !strings.Contains(m.View(), "Cannot") {
		t.Fatalf("missing actionable stop failure: %s", m.View())
	}
}
