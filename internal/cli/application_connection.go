package cli

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/maximilienGilet/sovereign-kit/internal/clientprofile"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/dashboardui"
)

type applicationConnection struct {
	generation uint64
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
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

func (m *applicationModel) beginConnection() tea.Cmd {
	if m.destroyedID > 0 {
		cfg, err := config.Load(m.path)
		if err == nil && cfg.Provider.Kind == "vast" && cfg.Provider.InstanceID == m.destroyedID {
			m.screen = "destroyed"
			return nil
		}
	}
	if m.connection != nil || m.session != nil {
		return nil
	}
	m.generation++
	ctx, cancel := context.WithCancel(m.ctx)
	c := &applicationConnection{generation: m.generation, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	m.connection = c
	m.screen = "connecting"
	m.status = "Opening tunnel and checking endpoint…"
	m.errText = ""
	go func() { defer close(c.done); c.tunnel, c.err = Connect(ctx, io.Discard, m.path, m.deps.Start) }()
	return func() tea.Msg { <-c.done; return applicationConnectionMsg{c.generation, "connected", c.err} }
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
		m.dashboard = dashboardui.NewEndpoint(c.ctx, endpoint, cfg.Provider.InstanceID, m.deps.Dashboard).SetHealthy(true)
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
