package dashboardui

import (
	"context"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/maximilienGilet/sovereign-kit/internal/clientprofile"
	"github.com/maximilienGilet/sovereign-kit/internal/endpointstats"
)

type Dependencies struct {
	Discover func(context.Context, string, clientprofile.Metadata) clientprofile.Endpoint
	Resolve  func(clientprofile.Integration) (clientprofile.Target, error)
	Inspect  func(context.Context, clientprofile.Target, clientprofile.Endpoint) clientprofile.Inspection
	Install  func(context.Context, clientprofile.Target, clientprofile.Endpoint, clientprofile.Inspection) (clientprofile.Inspection, error)
	Copy     func(string) error
	Stats    func(context.Context, string) endpointstats.Snapshot
}
type Model struct {
	ctx                           context.Context
	deps                          Dependencies
	endpoint                      clientprofile.Endpoint
	saved                         clientprofile.Metadata
	instance                      int
	healthy                       bool
	width, height, cursor, scroll int
	genericCursor                 int
	manualScroll                  bool
	mainNavigated                 bool
	session                       SessionInfo
	events                        []string
	stats                         endpointstats.Snapshot
	statsStarted                  bool
	statsEpoch                    uint64
	statsIdentity                 string
	throughput                    throughputHistory
	serverLogs                    string
	serverLogsAt                  time.Time
	logsExpanded                  bool
	page, status                  string
	target                        clientprofile.Target
	inspection                    clientprofile.Inspection
	confirm, busy                 bool
	operation                     uint64
	discoveryOperation            uint64
}

// SessionInfo contains only facts known by the caller. Nil price means unknown;
// StartedAt is the local connection start, not the remote server's boot time.
type SessionInfo struct {
	Provider    string
	GPU         string
	Region      string
	HourlyPrice *float64
	StartedAt   time.Time
}

func (m Model) WithSession(info SessionInfo) Model { m.session = info; return m }

// WithServerLogs supplies a previously fetched snapshot, never a live log stream.
func (m Model) WithServerLogs(text string, checkedAt time.Time) Model {
	m.serverLogs, m.serverLogsAt = text, checkedAt
	return m
}

type WorkResult struct {
	Operation  uint64
	Kind       string
	Endpoint   clientprofile.Endpoint
	Target     clientprofile.Target
	Inspection clientprofile.Inspection
	Err        error
}
type ClipboardResultMsg struct {
	Command   string
	Err       error
	Operation uint64
}

func NewEndpoint(ctx context.Context, endpoint clientprofile.Endpoint, instance int, deps Dependencies) Model {
	if ctx == nil {
		ctx = context.Background()
	}
	service := clientprofile.Service{}
	if deps.Discover == nil {
		deps.Discover = clientprofile.Discover
	}
	if deps.Resolve == nil {
		deps.Resolve = service.Resolve
	}
	if deps.Inspect == nil {
		deps.Inspect = service.Inspect
	}
	if deps.Install == nil {
		deps.Install = service.Install
	}
	if deps.Copy == nil {
		deps.Copy = clipboard.WriteAll
	}
	if deps.Stats == nil {
		deps.Stats = endpointstats.Read
	}
	saved := endpoint.Metadata
	if endpoint.Problem != "" {
		endpoint.Metadata = clientprofile.Metadata{}
	}
	return Model{ctx: ctx, deps: deps, endpoint: endpoint, saved: saved, instance: instance, page: "main", width: 80, height: 20, statsIdentity: endpoint.ID}
}
func (m *Model) record(event string) {
	m.events = append(append([]string(nil), m.events...), time.Now().Format("15:04:05")+"  "+event)
	if len(m.events) > 5 {
		m.events = m.events[len(m.events)-5:]
	}
}
func (m Model) SetHealthy(healthy bool) Model {
	wasHealthy := m.healthy
	if m.healthy != healthy {
		m.statsEpoch++
		if healthy {
			m.record("SSH tunnel connected")
		} else {
			m.record("SSH tunnel disconnected")
		}
	}
	m.healthy = healthy
	if healthy && !wasHealthy {
		m.throughput.reset()
	}
	if !healthy {
		m.stats = endpointstats.Snapshot{}
	}
	return m
}

type statsResultMsg struct {
	snapshot endpointstats.Snapshot
	epoch    uint64
}
type statsTickMsg struct{}

func (m Model) readStats() tea.Cmd {
	ctx, read, base, epoch := m.ctx, m.deps.Stats, m.endpoint.BaseURL, m.statsEpoch
	return func() tea.Msg {
		if ctx.Err() != nil {
			return nil
		}
		return statsResultMsg{snapshot: read(ctx, base), epoch: epoch}
	}
}

func (m Model) waitStats() tea.Cmd {
	ctx := m.ctx
	return func() tea.Msg {
		timer := time.NewTimer(3 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			return statsTickMsg{}
		}
	}
}
func (m Model) Init() tea.Cmd {
	if m.endpoint.BaseURL == "" {
		return nil
	}
	return m.discover()
}
func (m Model) discover() tea.Cmd {
	ctx, base, saved, discover, op := m.ctx, m.endpoint.BaseURL, m.saved, m.deps.Discover, m.discoveryOperation
	return func() tea.Msg {
		return WorkResult{Operation: op, Kind: "discover", Endpoint: discover(ctx, base, saved)}
	}
}
func (m Model) copy(value string) (tea.Model, tea.Cmd) {
	if value == "" {
		return m, nil
	}
	m.status = "Copying…"
	copy, op := m.deps.Copy, m.operation
	return m, func() tea.Msg { return ClipboardResultMsg{Command: value, Err: copy(value), Operation: op} }
}
func (m Model) inspect() (tea.Model, tea.Cmd) {
	if m.busy {
		return m, nil
	}
	kind := clientprofile.Pi
	if m.cursor == 4 {
		kind = clientprofile.OMP
	}
	if m.cursor == 5 {
		kind = clientprofile.OpenCode
	}
	m.operation++
	m.page = "inspecting"
	m.status = ""
	m.busy = true
	m.confirm = false
	m.scroll = 0
	deps, ctx, e, op := m.deps, m.ctx, m.endpoint, m.operation
	return m, func() tea.Msg {
		target, err := deps.Resolve(kind)
		if err != nil {
			return WorkResult{Operation: op, Kind: "inspect", Err: err}
		}
		return WorkResult{Operation: op, Kind: "inspect", Target: target, Inspection: deps.Inspect(ctx, target, e)}
	}
}
func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case statsResultMsg:
		if m.ctx.Err() != nil {
			return m, nil
		}
		if m.healthy && msg.epoch == m.statsEpoch {
			m.stats = msg.snapshot
			m.recordThroughput(msg.snapshot)
		}
		return m, m.waitStats()
	case statsTickMsg:
		if m.ctx.Err() != nil {
			return m, nil
		}
		if !m.healthy {
			return m, m.waitStats()
		}
		return m, m.readStats()
	case tea.WindowSizeMsg:
		m.width = max(12, msg.Width)
		m.height = max(3, msg.Height)
	case ClipboardResultMsg:
		if msg.Operation != m.operation {
			return m, nil
		}
		if msg.Err != nil {
			m.status = "Copy failed — copy the displayed value manually."
			m.record("Clipboard unavailable")
		} else {
			m.status = "Copied — paste in your application or project terminal."
			m.record("Copied to clipboard")
		}
	case WorkResult:
		if m.ctx.Err() != nil {
			return m, nil
		}
		if msg.Kind == "discover" {
			if msg.Operation != m.discoveryOperation {
				return m, nil
			}
			if m.endpoint.BaseURL != msg.Endpoint.BaseURL || (msg.Endpoint.ID != "" && m.statsIdentity != "" && m.statsIdentity != msg.Endpoint.ID) {
				m.throughput.reset()
				m.stats = endpointstats.Snapshot{}
				m.statsEpoch++
			}
			if msg.Endpoint.ID != "" {
				m.statsIdentity = msg.Endpoint.ID
			}
			m.endpoint = msg.Endpoint
			if m.endpoint.Problem == "" && m.endpoint.ID != "" {
				m.record("Model discovery verified")
			} else {
				m.record("Model discovery unavailable")
			}
			if !m.statsStarted && m.endpoint.BaseURL != "" {
				m.statsStarted = true
				return m, m.readStats()
			}
			return m, nil
		}
		if msg.Operation != m.operation {
			return m, nil
		}
		m.busy = false
		m.confirm = false
		m.scroll = 0
		if m.page != "inspecting" && m.page != "installing" {
			return m, nil
		}
		if msg.Err != nil {
			m.page = "error"
			m.status = msg.Err.Error()
			m.inspection = clientprofile.Inspection{}
			m.record("Integration setup failed")
			return m, nil
		}
		m.target = msg.Target
		m.inspection = msg.Inspection
		if !m.endpoint.Installable() && msg.Inspection.State != clientprofile.Unreadable {
			m.page = "blocked"
			m.status = "Profile installation unavailable. Model identity and verified limits are required. " + m.endpoint.Problem + " " + m.endpoint.LimitsProblem
			return m, nil
		}
		switch msg.Inspection.State {
		case clientprofile.Ready:
			m.page = "ready"
			m.record("Integration verified and ready")
		case clientprofile.Unreadable:
			m.page = "error"
			m.status = msg.Inspection.Detail
		default:
			if msg.Kind == "install" {
				m.page = "error"
				m.status = "Installation did not verify: " + msg.Inspection.Detail
			} else {
				m.page = "confirm"
				m.record("Integration inspected; awaiting confirmation")
			}
		}
	case tea.KeyMsg:
		if msg.Type == tea.KeyPgDown {
			m.manualScroll = true
			m.scroll += max(1, m.height-3)
			return m, nil
		}
		if msg.Type == tea.KeyPgUp {
			m.manualScroll = true
			m.scroll = max(0, m.scroll-max(1, m.height-3))
			return m, nil
		}
		m.manualScroll = false
		if msg.Type == tea.KeyEsc {
			m.page = "main"
			m.status = ""
			m.confirm = false
			m.scroll = 0
			return m, nil
		}
		if m.page == "main" && msg.String() == "l" {
			m.logsExpanded = !m.logsExpanded
			m.scroll = 0
			return m, nil
		}
		if !m.healthy || m.ctx.Err() != nil {
			return m, nil
		}
		switch m.page {
		case "main":
			switch msg.String() {
			case "down", "j":
				m.mainNavigated = true
				m.cursor = min(5, m.cursor+1)
				m.scroll = 0
			case "up", "k":
				m.mainNavigated = true
				m.cursor = max(0, m.cursor-1)
				m.scroll = 0
			case "r":
				if !m.busy {
					m.discoveryOperation++
					m.endpoint.Problem = "Checking endpoint…"
					return m, m.discover()
				}
			case "enter":
				switch m.cursor {
				case 0:
					return m.copy(m.endpoint.BaseURL)
				case 1:
					if m.endpoint.Problem == "" {
						return m.copy(m.endpoint.ID)
					}
				case 2:
					m.page = "generic"
					m.genericCursor = 0
					m.scroll = 0
				default:
					return m.inspect()
				}
			}
		case "generic":
			switch msg.String() {
			case "down", "j":
				m.genericCursor = min(2, m.genericCursor+1)
				m.scroll = 0
			case "up", "k":
				m.genericCursor = max(0, m.genericCursor-1)
				m.scroll = 0
			case "enter":
				switch m.genericCursor {
				case 0:
					return m.copy(m.endpoint.BaseURL)
				case 1:
					if m.endpoint.Problem == "" {
						return m.copy(m.endpoint.ID)
					}
				case 2:
					return m.copy("local-qwen-tunnel")
				}
			}
		case "confirm":
			switch msg.String() {
			case "left", "right", "tab", " ":
				m.confirm = !m.confirm
			case "enter":
				if !m.confirm {
					m.page = "main"
					return m, nil
				}
				m.page = "installing"
				m.busy = true
				m.status = ""
				deps, ctx, target, e, previous, op := m.deps, m.ctx, m.target, m.endpoint, m.inspection, m.operation
				return m, func() tea.Msg {
					inspection, err := deps.Install(ctx, target, e, previous)
					return WorkResult{Operation: op, Kind: "install", Target: target, Inspection: inspection, Err: err}
				}
			}
		case "ready":
			if msg.String() == "enter" {
				return m.copy(m.inspection.Command)
			}
		case "error", "blocked":
			if msg.String() == "r" {
				return m.inspect()
			}
		}
	}
	return m, nil
}

func (m *Model) recordThroughput(snapshot endpointstats.Snapshot) {
	if snapshot.Problem != "" && snapshot.CheckedAt.IsZero() {
		return
	}
	at := snapshot.CheckedAt
	if at.IsZero() {
		at = time.Now()
	}
	generation := snapshot.DecodeTokensPerSecond
	if snapshot.Problem != "" {
		generation = nil
	}
	m.throughput.add(at, generation)
}

func (m Model) ActionHint() string {
	switch m.page {
	case "confirm":
		if m.confirm {
			return "Install selected · ←→ · Enter confirm · Esc back"
		}
		return "Cancel selected · ←→ · Enter confirm · Esc back"
	case "ready":
		return "Enter copy command · Esc back"
	case "error", "blocked":
		return "r retry inspection · Esc back"
	case "installing", "inspecting":
		return "Working… · Esc back"
	case "generic":
		return "↑↓ select · Enter copy · Esc back"
	}
	if m.cursor < 2 {
		return "↑↓ select · Enter copy · l logs · r refresh · PgUp/PgDn scroll"
	}
	return "↑↓ select · Enter inspect · l logs · r refresh · PgUp/PgDn scroll"
}
