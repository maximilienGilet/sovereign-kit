package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
)

const recoveryTimeout = 2 * time.Minute

type applicationRecovery struct {
	action     string
	exit       bool
	generation uint64
	instanceID int
	cancel     context.CancelFunc
	done       chan struct{}
	err        error                  // published by done
	capability setup.InstanceRecovery // published by done
}
type applicationRecoveryMsg struct{ generation uint64 }

func (p *applicationPrompter) InstanceCreated(capability setup.InstanceRecovery) {
	if capability.InstanceID <= 0 || capability.Destroy == nil {
		return
	}
	s := p.session
	s.mu.Lock()
	s.recovery = capability
	s.mu.Unlock()
	select {
	case s.events <- capability:
	case <-s.ctx.Done():
	}
}
func (m *applicationModel) canDestroy() bool {
	return m.instanceID > 0 && m.recovery.InstanceID == m.instanceID && m.recovery.Destroy != nil && m.destruction == nil
}
func (m *applicationModel) beginDestruction() tea.Cmd {
	if !m.canDestroy() {
		return nil
	}
	return m.beginInstanceOperation("destroy", false)
}
func (m *applicationModel) beginInstanceOperation(action string, exit bool) tea.Cmd {
	capability := m.recovery
	perform := capability.Destroy
	if action == "stop" {
		perform = capability.Stop
	}
	if perform == nil || m.destruction != nil || capability.InstanceID != m.instanceID {
		return nil
	}
	session, connection := m.session, m.connection
	if session != nil {
		session.cancel()
	}
	if connection != nil {
		connection.cancel()
		connection.healthy = false
	}
	m.generation++
	ctx, cancel := context.WithTimeout(context.Background(), recoveryTimeout)
	operation := &applicationRecovery{action: action, exit: exit, generation: m.generation, instanceID: capability.InstanceID, cancel: cancel, done: make(chan struct{})}
	operation.capability = capability
	m.destruction = operation
	m.session = nil
	m.connection = nil // late events cannot overwrite the destruction screen
	m.pending = nil
	m.child = nil
	m.childGeneration++
	m.screen = "destroying"
	m.status = "Destroying and verifying instance…"
	if action == "stop" {
		m.status = "Stopping and verifying instance…"
	}
	go func() {
		defer close(operation.done)
		defer cancel()
		if session != nil {
			<-session.done
			session.mu.Lock()
			latest := session.recovery
			session.mu.Unlock()
			if latest.InstanceID == capability.InstanceID {
				fresh := latest.Destroy
				if action == "stop" {
					fresh = latest.Stop
				}
				if fresh != nil {
					perform = fresh
					operation.capability = latest
				}
			}
		}
		if connection != nil {
			stopApplicationConnection(connection)
		}
		if err := ctx.Err(); err != nil {
			operation.err = err
			return
		}
		operation.err = perform(ctx)
		if operation.err == nil && action == "destroy" {
			if err := retireDestroyedRoute(m.path, operation.instanceID); err != nil {
				operation.err = fmt.Errorf("instance #%d destruction confirmed, but saved connection cleanup failed: %w", operation.instanceID, err)
			}
		}
	}()
	return func() tea.Msg { <-operation.done; return applicationRecoveryMsg{operation.generation} }
}

// Keep a private, recoverable archive, but remove the route from the path read
// by subsequent application launches. Never retire another instance's route.
func retireDestroyedRoute(path string, instanceID int) error {
	cfg, err := config.Load(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if cfg.Provider.Kind != "vast" || cfg.Provider.InstanceID != instanceID {
		return nil
	}
	archive, err := os.MkdirTemp(filepath.Dir(path), fmt.Sprintf(".destroyed-%d-", instanceID))
	if err != nil {
		return err
	}
	if err := os.Rename(path, filepath.Join(archive, "config.toml")); err != nil {
		_ = os.Remove(archive) // Empty directory created above, never recursive.
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
func (m *applicationModel) finishDestruction(msg applicationRecoveryMsg) tea.Cmd {
	operation := m.destruction
	if operation == nil || msg.generation != operation.generation {
		return nil
	}
	select {
	case <-operation.done:
	default:
		return nil
	}
	if operation.capability.InstanceID == m.instanceID {
		m.recovery = operation.capability
	}
	if operation.err != nil {
		m.recoveryText = "Destruction unconfirmed: " + m.sanitize(operation.err.Error())
		if operation.action == "stop" {
			m.recoveryText = "Stop unconfirmed: " + m.sanitize(operation.err.Error())
		}
		m.screen = "error"
		if operation.exit {
			m.exitConfirm, m.exitDestroy = true, false
			m.exitError = m.recoveryText
			m.quitting = false
		}
	} else if operation.action == "stop" {
		m.stoppedID = operation.instanceID
		m.creating = false
	} else {
		m.destroyedID = operation.instanceID
		m.recoveryText = fmt.Sprintf("Instance #%d destroyed.\nAny saved route for this instance must not be reused.", operation.instanceID)
		m.recovery = setup.InstanceRecovery{}
		m.instanceID = 0
		m.creating = false
		m.loadConfiguration()
		m.screen = "destroyed"
	}
	m.destruction = nil
	if operation.exit && operation.err == nil {
		m.quitting = true
		return tea.Quit
	}
	if m.quitting {
		return tea.Quit
	}
	return nil
}
