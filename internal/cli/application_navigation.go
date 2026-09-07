package cli

import (
	"context"
	"fmt"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"io"
	"reflect"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

func (m *applicationModel) requestSetup() tea.Cmd {
	if m.locked || m.session != nil || m.connection != nil {
		return nil
	}
	if m.offerResume() {
		return nil
	}
	m.loadConfiguration()
	if m.exists {
		m.screen = "replace"
		m.selected = false
		return nil
	}
	return m.beginSetup()
}
func (m *applicationModel) beginSetup() tea.Cmd {
	if m.session != nil || m.locked {
		return nil
	}
	m.generation++
	m.serverLogs = setup.ServerLogs{}
	m.logsExpanded = false
	ctx, cancel := context.WithCancel(m.ctx)
	s := &applicationSession{generation: m.generation, ctx: ctx, cancel: cancel, events: make(chan any, 16), done: make(chan struct{})}
	m.session = s
	m.pending = nil
	m.child = nil
	m.screen = "working"
	m.status = "Starting setup…"
	m.errText = ""
	m.diagnostic.GotoTop()
	deps := m.deps.Setup
	deps.Prompter = &applicationPrompter{session: s}
	go func() { defer close(s.done); s.err = Setup(ctx, nil, io.Discard, m.path, m.defaultUser, deps) }()
	return s.next()
}

func (m *applicationModel) goBack() tea.Cmd {
	if m.locked || m.afterStop != "" {
		return nil
	}
	if m.pending != nil && m.editor != nil {
		switch m.pending.kind {
		case promptManual, promptHardware:
			m.drafts[m.pending.kind] = *m.editor
		default:
			if m.textInput() {
				m.drafts[m.pending.kind] = m.editor.text
			}
		}
	}
	// The current unaccepted prompt is not in history. Edit its predecessor.
	m.closeBrowser()
	target := len(m.history) - 1
	m.pending = nil
	m.child = nil
	m.confirmText = ""
	m.childGeneration++
	if target < 0 {
		m.history = nil
		m.replay = nil
		m.afterStop = "home"
	} else {
		m.replay = append([]promptAnswer(nil), m.history[:target]...)
		m.history = nil
		m.afterStop = "replay"
	}
	if m.session == nil {
		action := m.afterStop
		m.afterStop = ""
		if action == "replay" {
			return m.beginSetup()
		}
		m.screen = "home"
		return nil
	}
	m.session.cancel()
	m.screen = "working"
	m.status = "Cancelling previous operation…"
	// The existing event subscription drains to Done; only then can replay begin.
	return nil
}

func (m *applicationModel) textInput() bool {
	if m.screen != "prompt" || m.pending == nil {
		return false
	}
	if browser, ok := m.child.(*offerBrowserModel); ok {
		return browser.filtering
	}
	switch m.pending.kind {
	case promptManual, promptToken, promptIdentity, promptQuery, promptHardware, promptResumeID:
		return true
	}
	// Selects can accept search/filter text too.
	return m.pending.kind == promptModel || m.pending.kind == promptOffer
}
func (m *applicationModel) askExit() tea.Cmd {
	m.mergeProgress()
	m.exitInstance = m.instanceID
	m.exitChoice, m.exitError = 0, ""
	m.exitDestroy = false
	m.exitConfirm = true
	return nil
}
func (m *applicationModel) stopAndQuit() tea.Cmd {
	m.exitConfirm = false
	m.closeBrowser()
	if m.destruction != nil {
		m.quitting = true
		m.destruction.cancel()
		return nil // the subscribed completion joins before quitting
	}
	if m.session != nil {
		m.afterStop = "quit"
		m.session.cancel()
		m.pending = nil
		m.child = nil
		m.childGeneration++
		m.screen = "working"
		m.status = "Stopping local work…"
		return nil
	}
	if m.connection != nil {
		return m.stopConnection(true)
	}
	m.quitting = true
	return tea.Quit
}
func (m *applicationModel) paidWarning() string {
	if m.stoppedID > 0 && m.stoppedID == m.instanceID {
		return fmt.Sprintf("Vast instance %d: stop confirmed. Storage charges may remain. Reconnect to confirm its restart; the saved route is retained.", m.instanceID)
	}
	if m.destroyedID > 0 && m.instanceID == 0 && !m.creating {
		return ""
	}
	if m.instanceID > 0 {
		warning := fmt.Sprintf("Vast instance %d: billing may still be active. Exiting stops local resources only; check this instance in Vast.", m.instanceID)
		if m.recoveryText != "" {
			warning = m.recoveryText + "\n" + warning
		}
		return warning
	}
	if m.creating {
		return "Creation must be checked in Vast. The result may be uncertain and billing may be active. Do not create another instance blindly. Exiting does not stop a remote instance."
	}
	if m.locked {
		return "Setup crossed an approval/save boundary. Check its result before restarting. Exiting does not stop a remote instance."
	}
	return ""
}
func (m *applicationModel) compactPaidWarning() string {
	if m.instanceID > 0 {
		return fmt.Sprintf("Instance #%d\nWarning: billing may be active.", m.instanceID)
	}
	if m.creating {
		return "Creation must be checked in Vast.\nWarning: billing may be active."
	}
	return m.paidWarning()
}
func (m *applicationModel) setConfirmation(text string) {
	m.confirmText = text
	m.confirmation.SetContent(m.sanitize(text))
	m.confirmation.GotoTop()
	m.child = huh.NewForm(huh.NewGroup(huh.NewConfirm().Title("Approve?").Affirmative("Approve").Negative("Decline").Value(&m.editor.confirmed))).WithKeyMap(setupKeyMap()).WithShowHelp(false)
}

func (m *applicationModel) invalidateDependentDrafts(kind promptKind, value any) {
	previous, exists := m.drafts[kind]
	if !exists || reflect.DeepEqual(previous, value) {
		return
	}
	var dependent []promptKind
	switch kind {
	case promptProvider:
		dependent = []promptKind{promptWorkload, promptQuery, promptModel, promptHardware, promptCustom, promptOffer, promptBrowser, promptCost, promptIdentityConsent}
	case promptWorkload:
		dependent = []promptKind{promptQuery, promptModel, promptHardware, promptCustom, promptOffer, promptBrowser, promptCost}
	case promptQuery, promptModel:
		dependent = []promptKind{promptHardware, promptCustom, promptOffer, promptBrowser, promptCost}
		if kind == promptQuery {
			dependent = append(dependent, promptModel)
		}
	case promptHardware:
		dependent = []promptKind{promptCustom, promptOffer, promptBrowser, promptCost}
	case promptIdentity:
		dependent = []promptKind{promptIdentityConsent}
	case promptOffer, promptBrowser:
		dependent = []promptKind{promptCost}
	}
	for _, key := range dependent {
		delete(m.drafts, key)
		if key == promptBrowser {
			m.offerQuery = nil
		}
	}
}
