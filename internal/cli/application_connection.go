package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/maximilienGilet/sovereign-kit/internal/clientprofile"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/dashboardui"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
)

type applicationConnection struct {
	generation uint64
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	events     chan applicationReconnectMsg
	approval   chan bool
	tunnel     Tunnel
	err        error
	healthy    bool // root-thread only
	workers    sync.WaitGroup
	stopOnce   sync.Once
	failed     atomic.Bool
}
type applicationConnectionMsg struct {
	generation uint64
	phase      string
	err        error
}
type applicationDashboardMsg struct {
	generation uint64
	message    tea.Msg
}

type applicationReconnectMsg struct {
	generation uint64
	instanceID int
	status     string
	confirm    bool
}

func (c *applicationConnection) next() tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-c.events:
			return msg
		case <-c.done:
			return applicationConnectionMsg{c.generation, "connected", c.err}
		}
	}
}

func (m *applicationModel) reconnectMessage(msg applicationReconnectMsg) tea.Cmd {
	c := m.connection
	if c == nil || c.generation != msg.generation || c.ctx.Err() != nil {
		return nil
	}
	if msg.confirm {
		m.screen = "restart-confirm"
		m.selected = false
		m.instanceID = msg.instanceID
	} else {
		m.status = msg.status
		m.loader.stage(m.status, time.Now())
	}
	return c.next()
}

func (m *applicationModel) restartKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "left", "right", "up", "down", "tab", " ":
		m.selected = !m.selected
	case "esc", "n":
		return m.stopConnection(false)
	case "enter":
		if !m.selected {
			return m.stopConnection(false)
		}
		if c := m.connection; c != nil && c.ctx.Err() == nil {
			select {
			case c.approval <- true:
			default:
			}
			m.screen = "connecting"
			m.status = "Restarting instance…"
		}
	}
	return nil
}

func (m *applicationModel) beginConnection() tea.Cmd {
	if m.destroyedID > 0 {
		cfg, err := config.Load(m.path)
		if os.IsNotExist(err) || (err == nil && cfg.Provider.Kind == "vast" && cfg.Provider.InstanceID == m.destroyedID) {
			m.screen = "destroyed"
			return nil
		}
	}
	if m.connection != nil || m.session != nil {
		return nil
	}
	m.generation++
	// Retire any provisioning clock before the new connection generation starts.
	// Otherwise an active clock suppresses reconciliation while its old-generation
	// tick can no longer advance the verification animation.
	if m.loader.active {
		m.loader.active = false
		m.loader.epoch++
	}
	ctx, cancel := context.WithCancel(m.ctx)
	c := &applicationConnection{generation: m.generation, ctx: ctx, cancel: cancel, done: make(chan struct{}), events: make(chan applicationReconnectMsg), approval: make(chan bool, 1)}
	m.connection = c
	m.screen = "connecting"
	m.status = "Opening tunnel and checking endpoint…"
	m.loader.stageID = "verifying-connection"
	m.loader.serverPhase = ""
	if m.progressStage == setup.ProgressSaving && (len(m.loader.completed) == 0 || m.loader.completed[len(m.loader.completed)-1] != "Configuration saved") {
		m.loader.completed = append(m.loader.completed, "Configuration saved")
	}
	m.loader.future = nil
	m.loader.transferActive = false
	m.loader.transferCurrent = 0
	m.loader.transferTotal = 0
	m.loader.transferRatioHigh = 0
	m.loader.transferCompleted = false
	m.loader.transferCompletedAt = time.Time{}
	m.loader.providerStatus = ""
	m.loader.activity = nil
	m.loader.checkedAt = time.Time{}
	m.loader.activityChanged = time.Time{}
	m.loader.detailSnapshot = ""
	m.loader.providerMessage = ""
	m.loader.stage(m.status, time.Now())
	m.errText = ""
	prepare := m.reconnectPreparer()
	path, deps := m.path, m.deps.Start
	go func() {
		defer close(c.done)
		cfg, err := config.Load(path)
		if err != nil {
			c.err = err
			return
		}
		confirm := func(ctx context.Context, id int) (bool, error) {
			select {
			case c.events <- applicationReconnectMsg{generation: c.generation, instanceID: id, confirm: true}:
			case <-ctx.Done():
				return false, ctx.Err()
			}
			select {
			case yes := <-c.approval:
				return yes, nil
			case <-ctx.Done():
				return false, ctx.Err()
			}
		}
		progress := func(status string) {
			select {
			case c.events <- applicationReconnectMsg{generation: c.generation, status: status}:
			case <-ctx.Done():
			}
		}
		_, err = prepare(ctx, cfg, confirm, progress)
		if err != nil {
			c.err = err
			return
		}
		c.tunnel, c.err = Connect(ctx, io.Discard, path, deps)
	}()
	return c.next()
}
func (m *applicationModel) connectionMessage(msg applicationConnectionMsg) tea.Cmd {
	c := m.connection
	if c == nil || c.generation != msg.generation {
		return nil
	}
	if c.ctx.Err() != nil && msg.phase != "stopped" {
		return nil
	}
	switch msg.phase {
	case "stopped":
		m.connection = nil
		if m.quitting {
			return tea.Quit
		}
		if m.errText != "" {
			m.screen = "error"
		} else {
			m.screen = "home"
			m.loadConfiguration()
		}
		return nil
	case "connected":
		if msg.err != nil {
			m.errText = m.sanitize(msg.err.Error())
			m.screen = "error"
			m.cleanupConnection()
			return nil
		}
		cfg, err := config.Load(m.path)
		if err != nil {
			m.errText = m.sanitize(err.Error())
			return m.stopConnection(false)
		}
		c.healthy = true
		m.screen = "dashboard"
		endpoint := clientprofile.Endpoint{BaseURL: fmt.Sprintf("http://%s:%d/v1", cfg.Route.LocalHost, cfg.Route.LocalPort), Metadata: clientprofile.Metadata{ID: cfg.Model.ID, ContextWindow: cfg.Model.ContextWindow, MaxTokens: cfg.Model.MaxTokens}, Problem: "Checking model identity…"}
		m.dashboard = dashboardui.NewEndpoint(c.ctx, endpoint, cfg.Provider.InstanceID, m.deps.Dashboard).SetHealthy(true).WithSession(dashboardui.SessionInfo{Provider: cfg.Provider.Kind, StartedAt: time.Now()})
		m.dashboard = m.dashboard.WithServerLogs(m.serverLogs.Text, m.serverLogs.CheckedAt)
		m.resizeChild()
		watch := func() tea.Msg {
			select {
			case err := <-c.tunnel.Done():
				c.failed.Store(true)
				return applicationConnectionMsg{c.generation, "tunnel", tunnelExitError(err)}
			case <-c.ctx.Done():
				return nil
			}
		}
		return tea.Batch(watch, m.ownDashboardCommand(m.dashboard.Init()))
	case "tunnel":
		c.healthy = false
		m.dashboard = m.dashboard.SetHealthy(false)
		m.errText = m.sanitize(msg.err.Error())
		return m.stopConnection(false)
	}
	return nil
}

// Register and start work on the root event loop, before a disconnect can wait.
// The Bubble Tea command only delivers the joined worker's buffered result.
func (m *applicationModel) ownDashboardCommand(cmd tea.Cmd) tea.Cmd {
	c := m.connection
	if cmd == nil || c == nil || c.ctx.Err() != nil || c.failed.Load() {
		return nil
	}
	c.workers.Add(1)
	result := make(chan tea.Msg, 1)
	go func() {
		defer c.workers.Done()
		if c.ctx.Err() != nil {
			result <- nil
			return
		}
		result <- cmd()
	}()
	return func() tea.Msg { return applicationDashboardMsg{c.generation, <-result} }
}
func (m *applicationModel) dashboardUpdate(msg tea.Msg) tea.Cmd {
	c := m.connection
	if c == nil || !c.healthy || c.ctx.Err() != nil {
		return nil
	}
	if c.failed.Load() {
		c.healthy = false
		m.dashboard = m.dashboard.SetHealthy(false)
		return nil
	}
	select {
	case err := <-c.tunnel.Done():
		c.failed.Store(true)
		return m.connectionMessage(applicationConnectionMsg{c.generation, "tunnel", tunnelExitError(err)})
	default:
	}
	updated, cmd := m.dashboard.Update(msg)
	m.dashboard = updated.(dashboardui.Model)
	return m.ownDashboardCommand(cmd)
}
func updateDashboardSize(model dashboardui.Model, width, height int) (dashboardui.Model, tea.Cmd) {
	updated, cmd := model.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return updated.(dashboardui.Model), cmd
}
func (m *applicationModel) disconnect() tea.Cmd { return m.stopConnection(false) }
func (m *applicationModel) stopConnection(quit bool) tea.Cmd {
	c := m.connection
	if c == nil {
		if quit {
			m.quitting = true
			return tea.Quit
		}
		m.screen = "home"
		return nil
	}
	c.healthy = false
	c.cancel()
	m.quitting = quit
	m.screen = "connecting"
	m.status = "Stopping local tunnel and profile work…"
	return func() tea.Msg {
		stopApplicationConnection(c)
		return applicationConnectionMsg{c.generation, "stopped", nil}
	}
}
func stopApplicationConnection(c *applicationConnection) {
	c.stopOnce.Do(func() {
		c.cancel()
		<-c.done
		c.workers.Wait()
		if c.tunnel != nil {
			_ = c.tunnel.Stop()
		}
	})
}
func (m *applicationModel) cleanupConnection() {
	if m.connection != nil {
		stopApplicationConnection(m.connection)
		m.connection = nil
	}
}
