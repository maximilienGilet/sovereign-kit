package cli

import (
	"context"
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecoveryPrompterPreservesCapabilityOnCancelledDelivery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := &applicationSession{ctx: ctx, events: make(chan any), done: make(chan struct{})}
	p := &applicationPrompter{session: s}
	observer, ok := any(p).(setup.RecoveryObserver)
	if !ok {
		t.Fatal("application prompter loses instance recovery capability")
	}
	observer.InstanceCreated(setup.InstanceRecovery{InstanceID: 123, Destroy: func(context.Context) error { return nil }})
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	m.session = s
	m.mergeProgress()
	m.screen = "error"
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if m.screen != "destroy-confirm" {
		t.Fatal("cancellation lost session recovery")
	}
}

func TestRecoveryInvalidatesMatchingSavedRouteAndShutdownJoins(t *testing.T) {
	m := newApplication(context.Background(), startTestConfig(t), "root", "home", ApplicationDependencies{})
	cfg, err := config.Load(m.path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Provider = config.Provider{Kind: "vast", InstanceID: 456}
	if err := config.Save(m.path, cfg); err != nil {
		t.Fatal(err)
	}
	m.instanceID = 456
	m.creating = true
	m.locked = true
	m.screen = "error"
	m.recovery = setup.InstanceRecovery{InstanceID: 456, Destroy: func(context.Context) error { return nil }}
	deliverRecoveryCommand(t, m, m.beginDestruction())
	m.loadConfiguration()
	if m.configured {
		t.Fatal("destroyed saved route revived")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if strings.Contains(m.View(), "Check its result") || strings.Contains(m.View(), "billing may") {
		t.Fatal("verified destruction exit claims remote uncertainty")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.beginConnection() != nil || m.connection != nil {
		t.Fatal("connected to destroyed instance")
	}
	if _, err := config.Load(m.path); !os.IsNotExist(err) {
		t.Fatal("destroyed route remains active on disk")
	}
	m.instanceID = 789
	m.recovery = setup.InstanceRecovery{InstanceID: 789, Destroy: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }}
	cmd := m.beginDestruction()
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m.Update(tea.KeyMsg{Type: tea.KeyDown}) // explicit leave-running choice
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	deliverRecoveryCommand(t, m, cmd)
	if !m.quitting || m.destruction != nil || m.instanceID != 789 || !strings.Contains(m.recoveryText, "unconfirmed") {
		t.Fatalf("shutdown falsely confirmed cleanup: %+v", m)
	}
	if !strings.Contains(m.paidWarning(), "Destruction unconfirmed") {
		t.Fatal("restored terminal lost destruction uncertainty")
	}
}

func TestDestroyAndQuitRetiresRouteAcrossRestart(t *testing.T) {
	for _, tc := range []struct {
		name, action string
		id           int
		failure      bool
		retired      bool
	}{
		{"destroy", "destroy", 456, false, true},
		{"stop", "stop", 456, false, false},
		{"unconfirmed", "destroy", 456, true, false},
		{"different route", "destroy", 789, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := startTestConfig(t)
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			cfg.Provider = config.Provider{Kind: "vast", InstanceID: tc.id}
			if err := config.Save(path, cfg); err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			credential := filepath.Join(filepath.Dir(path), "vast-credentials")
			if err := os.WriteFile(credential, []byte("keep credential"), 0600); err != nil {
				t.Fatal(err)
			}
			m := newApplication(context.Background(), path, "root", "home", ApplicationDependencies{})
			m.instanceID = 456
			perform := func(context.Context) error {
				if tc.failure {
					return errors.New("provider unavailable")
				}
				return nil
			}
			m.recovery = setup.InstanceRecovery{InstanceID: 456, Destroy: perform, Stop: perform}
			deliverRecoveryCommand(t, m, m.beginInstanceOperation(tc.action, true))
			fresh := newApplication(context.Background(), path, "root", "home", ApplicationDependencies{})
			fresh.loadConfiguration()
			if fresh.configured == tc.retired {
				t.Fatalf("restart configured=%v, retired=%v", fresh.configured, tc.retired)
			}
			if tc.retired {
				archives, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".destroyed-456-*", "config.toml"))
				if err != nil || len(archives) != 1 {
					t.Fatalf("archive missing: %v %v", archives, err)
				}
				contents, err := os.ReadFile(archives[0])
				if err != nil || string(contents) != string(original) {
					t.Fatal("original route not recoverable")
				}
			}
			contents, err := os.ReadFile(credential)
			if err != nil || string(contents) != "keep credential" {
				t.Fatal("credential changed")
			}
		})
	}
}

func TestRecoveryWarningsStayPinnedWhenReadingError(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	m.screen = "error"
	m.locked = true
	m.instanceID = 456
	m.errText = strings.Repeat("long diagnostic ", 100)
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	m.View()
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if !strings.Contains(m.View(), "#456") || !strings.Contains(m.View(), "billing") {
		t.Fatal("scrolling lost paid identity/warning")
	}
	m.screen = "connecting"
	m.status = "Checking endpoint…"
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	if !strings.Contains(m.View(), "#456") || !strings.Contains(m.View(), "billing") {
		t.Fatal("connection lost paid identity/warning")
	}
}

type blockingRecoveryTunnel struct {
	applicationTunnel
	stopStarted, stopRelease chan struct{}
}

func (t *blockingRecoveryTunnel) Stop() error {
	close(t.stopStarted)
	<-t.stopRelease
	t.stops.Add(1)
	return nil
}

func TestRecoveryWaitsForConnectionHealthWorkerAndTunnelTeardown(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	ctx, cancel := context.WithCancel(context.Background())
	tunnel := &blockingRecoveryTunnel{applicationTunnel: applicationTunnel{done: make(chan error)}, stopStarted: make(chan struct{}), stopRelease: make(chan struct{})}
	c := &applicationConnection{generation: 3, ctx: ctx, cancel: cancel, done: make(chan struct{}), tunnel: tunnel, healthy: true}
	close(c.done)
	healthCancelled, healthRelease := make(chan struct{}), make(chan struct{})
	c.workers.Add(1)
	go func() { defer c.workers.Done(); <-ctx.Done(); close(healthCancelled); <-healthRelease }()
	destroyCalled := make(chan struct{})
	m.connection = c
	m.instanceID = 456
	m.screen = "error"
	m.recovery = setup.InstanceRecovery{InstanceID: 456, Destroy: func(context.Context) error { close(destroyCalled); return nil }}
	cmd := m.beginDestruction()
	<-healthCancelled
	select {
	case <-destroyCalled:
		t.Fatal("destroy preceded health worker join")
	case <-tunnel.stopStarted:
		t.Fatal("tunnel teardown raced health worker")
	case <-time.After(30 * time.Millisecond):
	}
	m.Update(applicationConnectionMsg{generation: 3, phase: "health"})
	if m.screen != "destroying" {
		t.Fatal("late health result overwrote cleanup")
	}
	close(healthRelease)
	select {
	case <-tunnel.stopStarted:
	case <-time.After(time.Second):
		t.Fatal("connection teardown never started")
	}
	select {
	case <-destroyCalled:
		t.Fatal("destroy preceded tunnel teardown")
	case <-time.After(30 * time.Millisecond):
	}
	close(tunnel.stopRelease)
	deliverRecoveryCommand(t, m, cmd)
	if tunnel.stops.Load() != 1 || m.screen != "destroyed" {
		t.Fatal("local teardown/destruction not completed exactly once")
	}
}

func TestRecoveryWithoutCapabilityNeverOffersDestruction(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	m.screen = "error"
	m.instanceID = 123
	m.errText = "instance 123 failed"
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if m.screen != "error" {
		t.Fatal("inferred destructive authority from ID")
	}
}

func TestRecoveryConfirmationAndUncertainRetry(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	ctx, cancel := context.WithCancel(context.Background())
	s := &applicationSession{ctx: ctx, cancel: cancel, events: make(chan any, 2), done: make(chan struct{})}
	p := &applicationPrompter{session: s}
	observer, ok := any(p).(setup.RecoveryObserver)
	if !ok {
		t.Fatal("missing recovery observer")
	}
	calls := 0
	observer.InstanceCreated(setup.InstanceRecovery{InstanceID: 456, Destroy: func(ctx context.Context) error {
		calls++
		if ctx.Err() != nil {
			t.Error("cleanup inherited cancelled provisioning")
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Error("cleanup is not bounded")
		}
		select {
		case <-s.done:
		default:
			t.Error("destruction preceded worker join")
		}
		if calls == 1 {
			return errors.New("provider secret-token unavailable")
		}
		return nil
	}})
	m.session = s
	m.mergeProgress()
	m.screen = "error"
	m.errText = "original setup failure"
	m.rememberSecret("secret-token")
	go func() { <-ctx.Done(); close(s.done) }()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if calls != 0 || m.screen != "error" {
		t.Fatal("default confirmation destroyed")
	}
	for attempt := 0; attempt < 2; attempt++ {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
		m.Update(tea.KeyMsg{Type: tea.KeyRight})
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		deliverRecoveryCommand(t, m, cmd)
		if attempt == 0 {
			if !strings.Contains(m.View(), "original setup failure") || strings.Contains(m.View(), "secret-token") || !strings.Contains(m.View(), "billing") {
				t.Fatalf("lost honest diagnostic: %s", m.View())
			}
		}
	}
	if calls != 2 || m.instanceID != 0 || !strings.Contains(m.View(), "#456 destroyed") {
		t.Fatalf("bad verified result: %s calls=%d", m.View(), calls)
	}
}

// Run every branch once, concurrently, as Tea does; don't drop worker events
// when an animation command is batched with the real operation.
func deliverRecoveryCommand(t *testing.T, m *applicationModel, cmd tea.Cmd) {
	t.Helper()
	updateApplicationCommand(t, m, cmd)
}

func updateApplicationCommand(t *testing.T, m *applicationModel, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return m, nil
	}
	var follow tea.Cmd
	var deliver func(tea.Msg)
	deliver = func(msg tea.Msg) {
		if batch, ok := msg.(tea.BatchMsg); ok {
			results := make(chan tea.Msg, len(batch))
			for _, c := range batch {
				go func(c tea.Cmd) { results <- c() }(c)
			}
			for range batch {
				select {
				case value := <-results:
					deliver(value)
				case <-time.After(3 * time.Second):
					t.Fatal("command blocked")
				}
			}
			return
		}
		_, next := m.Update(msg)
		if _, tick := msg.(provisioningTick); !tick && next != nil {
			follow = tea.Batch(follow, next)
		}
	}
	deliver(cmd())
	return m, follow
}
