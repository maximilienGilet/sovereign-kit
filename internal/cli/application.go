package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/catalogui"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/dashboardui"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

type ApplicationDependencies struct {
	Setup     SetupDependencies
	Start     StartDependencies
	Dashboard dashboardui.Dependencies
}

type applicationModel struct {
	ctx                         context.Context
	path, defaultUser, entry    string
	deps                        ApplicationDependencies
	width, height               int
	screen, status, errText     string
	configured, exists          bool
	generation, childGeneration uint64
	session                     *applicationSession
	child                       tea.Model
	resizeFormDescription       func(bool)
	pending                     *promptRequest
	editor                      *promptEditor
	history, replay             []promptAnswer
	drafts                      map[promptKind]any
	offerQuery                  *setup.OfferQuery
	locked, creating            bool
	instanceID                  int
	quitting, exitConfirm       bool
	afterStop                   string
	confirmText                 string
	confirmation                viewport.Model
	recipeViewport              viewport.Model
	diagnostic                  viewport.Model
	selected                    bool
	secrets                     []string
	connection                  *applicationConnection
	dashboard                   dashboardui.Model
	loader                      provisioningLoader
	recovery                    setup.InstanceRecovery
	destruction                 *applicationRecovery
	recoveryText                string
	destroyedID                 int
	progressStage               string
	resumeID                    int
	resumeDestroy               bool
	serverLogs                  setup.ServerLogs
	logViewport                 viewport.Model
	logsExpanded                bool
}
type applicationBegin struct{}
type applicationChildMsg struct {
	generation uint64
	message    tea.Msg
}

func newApplication(ctx context.Context, path, user, entry string, deps ApplicationDependencies) *applicationModel {
	if ctx == nil {
		ctx = context.Background()
	}
	if deps.Setup.Getenv == nil {
		deps.Setup.Getenv = os.Getenv
	}
	token := deps.Setup.Getenv("VAST_API_KEY")
	getenv := deps.Setup.Getenv
	deps.Setup.Getenv = func(key string) string {
		if key == "VAST_API_KEY" {
			return token
		}
		return getenv(key)
	}
	m := &applicationModel{ctx: ctx, path: path, defaultUser: user, entry: entry, deps: deps, width: 80, height: 24, screen: "home", drafts: make(map[promptKind]any), confirmation: viewport.New(80, 16), recipeViewport: viewport.New(80, 20), diagnostic: viewport.New(80, 20)}
	m.rememberSecret(token)
	m.logViewport = viewport.New(80, 16)
	m.loadConfiguration()
	return m
}
func RunApplication(ctx context.Context, input io.Reader, output io.Writer, path, user, entry string, deps ApplicationDependencies) error {
	if entry != "home" && entry != "setup" && entry != "start" && entry != "resume" && !strings.HasPrefix(entry, "resume:") {
		return fmt.Errorf("unknown application entry %q", entry)
	}
	if input == nil {
		input = os.Stdin
	}
	if output == nil {
		output = os.Stdout
	}
	m := newApplication(ctx, path, user, entry, deps)
	_, err := tea.NewProgram(m, tea.WithContext(m.ctx), tea.WithInput(input), tea.WithOutput(output), tea.WithAltScreen()).Run()
	m.cleanup()
	if m.creating || m.instanceID > 0 {
		fmt.Fprintln(output, m.paidWarning())
	}
	return err
}
func (m *applicationModel) Init() tea.Cmd { return func() tea.Msg { return applicationBegin{} } }
func (m *applicationModel) Update(msg tea.Msg) (model tea.Model, cmd tea.Cmd) {
	defer func() { cmd = tea.Batch(cmd, m.reconcileLoader()) }()
	switch msg := msg.(type) {
	case provisioningTick:
		return m, m.updateLoader(msg)
	case applicationRecoveryMsg:
		return m, m.finishDestruction(msg)
	case applicationBegin:
		if m.offerResume() {
			return m, nil
		}
		if m.entry == "setup" {
			return m, m.requestSetup()
		}
		if m.entry == "start" {
			return m, m.beginConnection()
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeChild()
		return m, nil
	case applicationEvent:
		if m.session == nil || msg.generation != m.session.generation {
			return m, nil
		}
		switch value := msg.value.(type) {
		case setup.InstanceRecovery:
			m.recovery = value
			m.instanceID = value.InstanceID
			m.creating, m.locked = true, true
			return m, m.session.next()
		case promptRequest:
			if m.afterStop != "" {
				return m, m.session.next()
			}
			if len(m.replay) > 0 && validReplay(value, m.replay[0]) {
				answer := m.replay[0]
				m.replay = m.replay[1:]
				m.history = append(m.history, answer)
				// Return the fresh offer object, never stale price/hardware data.
				if value.kind == promptOffer {
					for _, view := range value.data.([]setup.OfferView) {
						if view.Offer.ID == answer.value.(vast.Offer).ID {
							answer.value = view.Offer
							break
						}
					}
				}
				value.reply <- answer.value
				return m, m.session.next()
			}
			m.replay = nil
			return m, tea.Batch(m.openPrompt(value), m.session.next())
		case setup.Progress:
			m.applyProgress(value)
			return m, m.session.next()
		case setup.Activity:
			m.applyActivity(value)
			return m, m.session.next()
		case setup.ServerLogs:
			m.applyServerLogs(value)
			return m, m.session.next()
		case applicationDone:
			m.mergeProgress()
			m.closeBrowser()
			m.session = nil
			m.pending = nil
			m.child = nil
			if m.afterStop != "" {
				action := m.afterStop
				m.afterStop = ""
				if action == "replay" {
					return m, m.beginSetup()
				}
				if action == "quit" {
					m.quitting = true
					return m, tea.Quit
				}
				m.screen = "home"
				return m, nil
			}
			if value.err != nil {
				m.errText = m.sanitize(value.err.Error())
				m.screen = "error"
			} else if m.resumeDestroy {
				m.resumeDestroy = false
				if m.canDestroy() {
					m.screen = "destroy-confirm"
					m.selected = false
				} else {
					m.screen = "error"
					m.errText = "No verified destruction capability is available."
				}
			} else {
				m.screen = "saved"
				m.loadConfiguration()
				m.status = "Configuration saved — route not verified"
			}
		}
		return m, nil
	case applicationChildMsg:
		if msg.generation != m.childGeneration || m.child == nil || m.afterStop != "" {
			return m, nil
		}
		if m.exitConfirm {
			// Preserve completed read-only search data, but do not advance
			// hidden animations/forms or schedule their next timer.
			if _, ok := msg.message.(browserSearchMsg); ok {
				m.updateChild(msg.message)
			}
			return m, nil
		}
		return m, m.updateChild(msg.message)
	case applicationConnectionMsg:
		return m, m.connectionMessage(msg)
	case applicationDashboardMsg:
		if m.screen == "dashboard" && m.connection != nil && m.connection.generation == msg.generation {
			return m, m.dashboardUpdate(msg.message)
		}
		return m, nil
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, m.askExit()
		}
		if m.exitConfirm {
			switch msg.String() {
			case "y":
				return m, m.stopAndQuit()
			case "n", "esc":
				m.exitConfirm = false
			}
			return m, nil
		}
		if (m.width < 30 || m.height < 10) && m.screen != "dashboard" {
			return m, nil
		}
		if m.afterStop != "" {
			return m, nil
		}
		if m.logsExpanded {
			if msg.String() == "l" || msg.String() == "esc" {
				m.logsExpanded = false
				return m, nil
			}
			m.logViewport, cmd = m.logViewport.Update(msg)
			return m, cmd
		}
		if msg.String() == "l" && (m.screen == "working" || m.screen == "error" || m.screen == "saved") && (m.progressStage == setup.ProgressLaunching || !m.serverLogs.CheckedAt.IsZero()) {
			m.logsExpanded = true
			m.logViewport.GotoBottom()
			return m, nil
		}
		if msg.String() == "q" && !m.textInput() {
			return m, m.askExit()
		}
		switch m.screen {
		case "resume":
			switch msg.String() {
			case "enter":
				return m, m.beginResume(false)
			case "d":
				return m, m.beginResume(true)
			}
		case "home":
			switch msg.String() {
			case "i":
				m.entry = "resume"
				m.offerResume()
				return m, nil
			case "enter":
				if m.configured {
					return m, m.beginConnection()
				}
				return m, m.requestSetup()
			case "s", "r":
				return m, m.requestSetup()
			case "c":
				if m.configured {
					return m, m.beginConnection()
				}
			}
		case "replace":
			switch msg.String() {
			case "left", "right", " ":
				m.selected = !m.selected
			case "y":
				m.selected = true
				return m, m.beginSetup()
			case "n", "esc":
				m.screen = "home"
			case "enter":
				if m.selected {
					return m, m.beginSetup()
				}
				m.screen = "home"
			}
		case "prompt":
			if m.pending != nil && m.pending.kind == promptWorkload && (msg.Type == tea.KeyPgUp || msg.Type == tea.KeyPgDown) {
				if picker, ok := m.child.(catalogui.Model); ok && picker.EvidenceOpen() {
					return m, m.updateChild(msg)
				}
				m.recipeViewport, _ = m.recipeViewport.Update(msg)
				return m, nil
			}
			if msg.Type == tea.KeyEsc {
				if _, ok := m.child.(*offerBrowserModel); ok {
					return m, m.updateChild(msg)
				}
				if _, ok := m.child.(catalogui.Model); ok {
					return m, m.updateChild(msg)
				}
				return m, m.goBack()
			}
			return m, m.updateChild(msg)
		case "saved":
			if msg.String() == "enter" || msg.String() == "c" {
				return m, m.beginConnection()
			}
			if msg.Type == tea.KeyEsc {
				m.screen = "home"
			}
		case "error":
			if msg.String() == "d" && m.canDestroy() {
				m.selected = false
				m.screen = "destroy-confirm"
				return m, nil
			}
			if msg.Type == tea.KeyPgUp || msg.Type == tea.KeyPgDown {
				m.diagnostic, _ = m.diagnostic.Update(msg)
				return m, nil
			}
			if msg.Type == tea.KeyEsc && !m.locked {
				return m, m.goBack()
			}
		case "dashboard":
			if msg.String() == "d" {
				return m, m.disconnect()
			}
			return m, m.dashboardUpdate(msg)
		case "connecting":
			if msg.Type == tea.KeyEsc {
				return m, m.disconnect()
			}
		case "destroy-confirm":
			switch msg.String() {
			case "left", "right", " ":
				m.selected = !m.selected
			case "esc", "n":
				m.screen = "error"
			case "enter":
				if m.selected {
					return m, m.beginDestruction()
				}
				m.screen = "error"
			}
		}
	}
	return m, nil
}

func (m *applicationModel) updateChild(msg tea.Msg) tea.Cmd {
	if m.pending == nil || m.child == nil {
		return nil
	}
	switch result := msg.(type) {
	case browserSelectedMsg:
		return m.acceptPrompt(result.offer)
	case browserBackMsg:
		return m.goBack()
	case catalogui.PickerResultMsg:
		if result.Cancelled {
			return m.goBack()
		}
		return m.acceptPrompt(result.Value)
	case rentalReviewResultMsg:
		return m.acceptPrompt(result.Confirmed)
	}
	// Long confirmation text scrolls independently; approval is always explicit.
	if m.confirmText != "" {
		if key, ok := msg.(tea.KeyMsg); ok && (key.String() == "pgup" || key.String() == "pgdown") {
			m.confirmation, _ = m.confirmation.Update(key)
			return nil
		}
	}
	evidenceWasOpen := false
	if picker, ok := m.child.(catalogui.Model); ok {
		evidenceWasOpen = picker.EvidenceOpen()
	}
	child, cmd := m.child.Update(msg)
	m.child = child
	if picker, ok := child.(catalogui.Model); ok && picker.EvidenceOpen() != evidenceWasOpen {
		m.resizeChild()
	}
	if form, ok := child.(*huh.Form); ok && form.State == huh.StateCompleted {
		answer, err := m.editorAnswer()
		if err != nil {
			m.errText = m.sanitize(err.Error())
			return nil
		}
		return m.acceptPrompt(answer)
	}
	return m.tagChild(cmd)
}
func (m *applicationModel) tagChild(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	generation := m.childGeneration
	return func() tea.Msg {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			commands := make([]tea.Cmd, len(batch))
			for i, child := range batch {
				commands[i] = tagApplicationChild(generation, child)
			}
			return tea.BatchMsg(commands)
		}
		return applicationChildMsg{generation, msg}
	}
}
func tagApplicationChild(generation uint64, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for i, child := range batch {
				batch[i] = tagApplicationChild(generation, child)
			}
			return batch
		}
		return applicationChildMsg{generation, msg}
	}
}
func (m *applicationModel) acceptPrompt(value any) tea.Cmd {
	if m.pending == nil || m.session == nil || m.afterStop != "" {
		return nil
	}
	request := m.pending
	m.closeBrowser()
	m.invalidateDependentDrafts(request.kind, value)
	if request.kind == promptToken {
		m.rememberSecret(value.(string))
	}
	if request.kind == promptManual || ((request.kind == promptCost || request.kind == promptIdentityConsent) && value == true) {
		m.locked = true
	}
	m.history = append(m.history, promptAnswer{request.kind, value})
	m.drafts[request.kind] = value
	m.pending = nil
	m.child = nil
	m.editor = nil
	m.confirmText = ""
	m.childGeneration++
	m.screen = "working"
	m.status = "Working…"
	request.reply <- value
	return nil
}
func (m *applicationModel) loadConfiguration() {
	_, statErr := os.Lstat(m.path)
	m.exists = !os.IsNotExist(statErr)
	cfg, err := config.Load(m.path)
	m.configured = err == nil
	if err == nil && m.destroyedID > 0 && cfg.Provider.Kind == "vast" && cfg.Provider.InstanceID == m.destroyedID {
		m.configured = false
	}
	if err != nil && m.exists {
		m.errText = m.sanitize(err.Error())
	} else {
		m.errText = ""
	}
}
func (m *applicationModel) rememberSecret(value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		m.secrets = append(m.secrets, value)
	}
}
func (m *applicationModel) sanitize(value string) string {
	for _, secret := range m.secrets {
		value = strings.ReplaceAll(value, secret, "[redacted]")
	}
	value = ansi.Strip(value)
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r >= 32 && r != 127 {
			return r
		}
		return -1
	}, value)
	runes := []rune(value)
	if len(runes) > 2000 {
		value = string(runes[:2000]) + "…"
	}
	return value
}
func (m *applicationModel) applyProgress(p setup.Progress) {
	if p.Stage == "" {
		return
	}
	if p.Stage != setup.ProgressWaiting {
		m.loader.activity = nil
		m.loader.checkedAt = time.Time{}
		m.loader.providerStatus = ""
	}
	if m.progressStage != "" && p.Stage != m.progressStage {
		completed := ""
		switch {
		case m.progressStage == setup.ProgressCreating && (p.Stage == setup.ProgressCreated || p.Stage == setup.ProgressWaiting):
			completed = "Instance created"
		case m.progressStage == setup.ProgressWaiting && p.Stage == setup.ProgressHostKeys:
			completed = "Instance running"
		case m.progressStage == setup.ProgressHostKeys && p.Stage == setup.ProgressLaunching:
			completed = "Host fingerprints verified"
		case m.progressStage == setup.ProgressLaunching && p.Stage == setup.ProgressSaving:
			completed = "Server launched"
		}
		if completed != "" {
			m.loader.completed = append(m.loader.completed, completed)
		}
	}
	m.progressStage = p.Stage
	m.loader.future = nil
	stages := []string{setup.ProgressCreating, setup.ProgressWaiting, setup.ProgressHostKeys, setup.ProgressLaunching, setup.ProgressSaving}
	labels := []string{"Create instance", "Wait for instance", "Verify host fingerprints", "Launch inference server", "Save configuration"}
	stage := p.Stage
	if stage == setup.ProgressCreated {
		stage = setup.ProgressCreating
	}
	for i, known := range stages {
		if stage == known {
			m.loader.future = append([]string(nil), labels[i+1:]...)
			m.loader.future = append(m.loader.future, "Verify connection")
			break
		}
	}
	m.status = progressLabel(p.Stage)
	if p.Stage == setup.ProgressCreating {
		m.creating = true
		m.locked = true
	}
	if p.InstanceID > 0 {
		m.instanceID = p.InstanceID
		m.creating = true
		m.locked = true
	}
}
func (m *applicationModel) mergeProgress() {
	if m.session == nil {
		return
	}
	m.session.mu.Lock()
	defer m.session.mu.Unlock()
	if m.session.creating {
		m.creating = true
	}
	if m.session.recovery.InstanceID > 0 {
		m.recovery = m.session.recovery
		m.instanceID = m.recovery.InstanceID
		m.creating, m.locked = true, true
	}
	m.applyProgress(m.session.progress)
	m.applyActivity(m.session.activity)
	m.applyServerLogs(m.session.logs)
}
func (m *applicationModel) cleanup() {
	if m.destruction != nil {
		m.destruction.cancel()
		<-m.destruction.done
		m.finishDestruction(applicationRecoveryMsg{m.destruction.generation})
	}
	m.closeBrowser()
	if m.session != nil {
		m.session.cancel()
		<-m.session.done
		m.mergeProgress()
		m.session = nil
	}
	m.cleanupConnection()
}

func (m *applicationModel) closeBrowser() {
	if browser, ok := m.child.(*offerBrowserModel); ok {
		query := browser.query
		query.Countries = append([]string(nil), query.Countries...)
		m.offerQuery = &query
		browser.Close()
	}
}
