package cli

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/maximilienGilet/sovereign-kit/internal/clientprofile"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/dashboardui"
)

type applicationTunnel struct {
	done  chan error
	stops atomic.Int32
}

func (t *applicationTunnel) Start() error       { return nil }
func (t *applicationTunnel) Done() <-chan error { return t.done }
func (t *applicationTunnel) Stop() error        { t.stops.Add(1); return nil }
func connectionApplication(t *testing.T, deps dashboardui.Dependencies) (*applicationModel, *applicationTunnel, tea.Cmd) {
	t.Helper()
	tunnel := &applicationTunnel{done: make(chan error, 1)}
	if deps.Discover == nil {
		deps.Discover = func(_ context.Context, base string, _ clientprofile.Metadata) clientprofile.Endpoint {
			return clientprofile.Endpoint{BaseURL: base, Metadata: clientprofile.Metadata{ID: "owner/solo", ContextWindow: 32768, MaxTokens: 4096}}
		}
	}
	m := newApplication(context.Background(), startTestConfig(t), "root", "start", ApplicationDependencies{Dashboard: deps, Start: StartDependencies{NewTunnel: func(context.Context, config.Config, io.Writer) (Tunnel, error) { return tunnel, nil }, Healthcheck: func(context.Context, string) error { return nil }}})
	t.Cleanup(m.cleanup)
	_, cmd := m.Update(m.Init()())
	return m, tunnel, cmd
}
func connectedDashboard(t *testing.T, m *applicationModel, connect tea.Cmd) tea.Cmd {
	t.Helper()
	_, batch := updateApplicationCommand(t, m, connect)
	commands, ok := batch().(tea.BatchMsg)
	if !ok || len(commands) != 2 {
		t.Fatal("discovery and tunnel watcher not scheduled")
	}
	m.Update(commands[1]())
	return commands[0]
}
func TestApplicationEndpointCopyAndEscapeKeepTunnelOpen(t *testing.T) {
	copied := ""
	m, tunnel, connect := connectionApplication(t, dashboardui.Dependencies{Copy: func(v string) error { copied = v; return nil }})
	watch := connectedDashboard(t, m, connect)
	if !strings.Contains(m.View(), "owner/solo") {
		t.Fatal("actual model missing")
	}
	_, copy := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(copy())
	if copied != "http://127.0.0.1:30000/v1" || !strings.Contains(m.View(), "Copied") {
		t.Fatal("raw endpoint copy failed")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.screen != "dashboard" || tunnel.stops.Load() != 0 {
		t.Fatal("escape disconnected")
	}
	_, stop := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	updateApplicationCommand(t, m, stop)
	if m.screen != "home" || tunnel.stops.Load() != 1 || watch() != nil {
		t.Fatal("disconnect failed")
	}
}
func TestApplicationDisconnectCancelsAndJoinsProfileWorkerIgnoringLateResult(t *testing.T) {
	started, finished := make(chan struct{}), make(chan struct{})
	m, tunnel, connect := connectionApplication(t, dashboardui.Dependencies{Resolve: func(k clientprofile.Integration) (clientprofile.Target, error) {
		return clientprofile.Target{k, "/tmp/fixture"}, nil
	}, Inspect: func(ctx context.Context, _ clientprofile.Target, _ clientprofile.Endpoint) clientprofile.Inspection {
		close(started)
		<-ctx.Done()
		close(finished)
		return clientprofile.Inspection{State: clientprofile.Ready, Command: "stale command"}
	}})
	connectedDashboard(t, m, connect)
	for i := 0; i < 3; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	_, inspect := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("inspection worker not owned immediately")
	}
	_, stop := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	updateApplicationCommand(t, m, stop)
	select {
	case <-finished:
	default:
		t.Fatal("disconnect did not join worker")
	}
	m.Update(inspect())
	if m.screen != "home" || m.connection != nil || tunnel.stops.Load() != 1 || strings.Contains(m.View(), "stale command") {
		t.Fatal("stale worker revived connection")
	}
}
func TestApplicationQueuedTunnelLossCannotBeRevivedByDiscovery(t *testing.T) {
	m, tunnel, connect := connectionApplication(t, dashboardui.Dependencies{})
	_, batch := updateApplicationCommand(t, m, connect)
	commands := batch().(tea.BatchMsg)
	result := make(chan tea.Msg, 1)
	go func() { result <- commands[0]() }()
	tunnel.done <- errors.New("SSH lost")
	lost := <-result
	m.Update(commands[1]())
	_, stop := m.Update(lost)
	if stop != nil {
		updateApplicationCommand(t, m, stop)
	}
	if tunnel.stops.Load() != 1 || !strings.Contains(m.View(), "SSH lost") {
		t.Fatal("tunnel loss not retained")
	}
}

func TestSmallDashboardKeepsCopyAndDisconnectAvailable(t *testing.T) {
	copied := ""
	m, _, connect := connectionApplication(t, dashboardui.Dependencies{Copy: func(v string) error { copied = v; return nil }})
	connectedDashboard(t, m, connect)
	m.Update(tea.WindowSizeMsg{Width: 24, Height: 8})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || strings.Contains(m.View(), "Enlarge") {
		t.Fatal("small dashboard lost actions")
	}
	m.Update(cmd())
	if copied != "http://127.0.0.1:30000/v1" {
		t.Fatal("small dashboard could not copy")
	}
}
