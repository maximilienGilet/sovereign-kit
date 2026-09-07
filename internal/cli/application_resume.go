package cli

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"io"
	"os"
	"strconv"
	"strings"
)

const promptResume promptKind = "Resume instance"
const promptResumeID promptKind = "Recover existing instance"

func (p *applicationPrompter) ResumeInstanceID(ctx context.Context) (int, error) {
	value, err := promptValue[string](p, ctx, promptResumeID, nil)
	if err != nil {
		return 0, err
	}
	return parseResumeID(value)
}

func (p *applicationPrompter) ConfirmResume(ctx context.Context, id int, r recipe.Recipe, identity string) (bool, error) {
	return promptValue[bool](p, ctx, promptResume, fmt.Sprintf("Resume instance #%d?\nRecipe: %s\nModel: %s\nRevision: %s\nSSH identity: %s\n\nNo new instance will be created. Billing may still be active.", id, r.Name, r.Model.Repository, r.Model.Revision, identity))
}

// Pending state takes precedence over configured routes and new setup.
func (m *applicationModel) offerResume() bool {
	cp, err := setup.ReadCheckpoint(setup.CheckpointPath(m.path))
	explicit := m.entry == "resume" || strings.HasPrefix(m.entry, "resume:")
	if err != nil && !os.IsNotExist(err) {
		m.screen, m.errText, m.locked = "error", "Pending deployment cannot be read: "+m.sanitize(err.Error()), true
		return true
	}
	if os.IsNotExist(err) && !explicit {
		return false
	}
	m.resumeID = 0
	if strings.HasPrefix(m.entry, "resume:") {
		id, parseErr := strconv.Atoi(strings.TrimPrefix(m.entry, "resume:"))
		if parseErr != nil || id <= 0 {
			m.screen, m.errText, m.locked = "error", "A positive instance ID is required.", true
			return true
		}
		if err == nil && cp.InstanceID > 0 && id != cp.InstanceID {
			m.screen, m.errText, m.locked = "error", "Requested instance does not match pending deployment.", true
			return true
		}
		m.resumeID = id
	}
	m.screen = "resume"
	m.instanceID = cp.InstanceID
	if m.resumeID > 0 {
		m.instanceID = m.resumeID
	}
	m.creating, m.locked = true, true
	if os.IsNotExist(err) && m.resumeID == 0 {
		m.creating = false
		m.status = "No local checkpoint — recover by instance ID"
		return true
	}
	m.status = "Existing deployment"
	if err == nil {
		m.status = cp.Recipe.Name + " · " + cp.Phase
	}
	return true
}

func (m *applicationModel) beginResume(recoveryOnly bool) tea.Cmd {
	if m.session != nil || m.connection != nil {
		return nil
	}
	m.generation++
	m.serverLogs = setup.ServerLogs{}
	m.logsExpanded = false
	ctx, cancel := context.WithCancel(m.ctx)
	s := &applicationSession{generation: m.generation, ctx: ctx, cancel: cancel, events: make(chan any, 16), done: make(chan struct{})}
	m.session = s
	m.pending, m.child = nil, nil
	m.resumeDestroy = recoveryOnly
	m.screen, m.status, m.errText = "working", "Checking existing deployment…", ""
	deps := m.deps.Setup
	deps.Prompter = &applicationPrompter{session: s}
	id := m.resumeID
	go func() { defer close(s.done); s.err = ResumeSetup(ctx, io.Discard, m.path, deps, id, recoveryOnly) }()
	return s.next()
}
