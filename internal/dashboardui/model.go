package dashboardui

import (
	"context"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/maximilienGilet/sovereign-kit/internal/clientprofile"
)

type Dependencies struct {
	Discover func(context.Context, string, clientprofile.Metadata) clientprofile.Endpoint
	Resolve  func(clientprofile.Integration) (clientprofile.Target, error)
	Inspect  func(context.Context, clientprofile.Target, clientprofile.Endpoint) clientprofile.Inspection
	Install  func(context.Context, clientprofile.Target, clientprofile.Endpoint, clientprofile.Inspection) (clientprofile.Inspection, error)
	Copy     func(string) error
}
type Model struct {
	ctx                           context.Context
	deps                          Dependencies
	endpoint                      clientprofile.Endpoint
	saved                         clientprofile.Metadata
	instance                      int
	healthy                       bool
	width, height, cursor, scroll int
	page, status                  string
	target                        clientprofile.Target
	inspection                    clientprofile.Inspection
	confirm, busy                 bool
	operation                     uint64
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
	saved := endpoint.Metadata
	if endpoint.Problem != "" {
		endpoint.Metadata = clientprofile.Metadata{}
	}
	return Model{ctx: ctx, deps: deps, endpoint: endpoint, saved: saved, instance: instance, page: "main", width: 80, height: 20}
}
func (m Model) SetHealthy(healthy bool) Model { m.healthy = healthy; return m }
func (m Model) Init() tea.Cmd {
	if m.endpoint.BaseURL == "" {
		return nil
	}
	return m.discover()
}
func (m Model) discover() tea.Cmd {
	ctx, base, saved, discover, op := m.ctx, m.endpoint.BaseURL, m.saved, m.deps.Discover, m.operation
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
	case tea.WindowSizeMsg:
		m.width = max(12, msg.Width)
		m.height = max(3, msg.Height)
	case ClipboardResultMsg:
		if msg.Operation != m.operation {
			return m, nil
		}
		if msg.Err != nil {
			m.status = "Copy failed — copy the displayed value manually."
		} else {
			m.status = "Copied — paste in your application or project terminal."
		}
	case WorkResult:
		if msg.Operation != m.operation || m.ctx.Err() != nil {
			return m, nil
		}
		if msg.Kind == "discover" {
			m.endpoint = msg.Endpoint
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
		case clientprofile.Unreadable:
			m.page = "error"
			m.status = msg.Inspection.Detail
		default:
			if msg.Kind == "install" {
				m.page = "error"
				m.status = "Installation did not verify: " + msg.Inspection.Detail
			} else {
				m.page = "confirm"
			}
		}
	case tea.KeyMsg:
		if msg.Type == tea.KeyPgDown {
			m.scroll += max(1, m.height-3)
			return m, nil
		}
		if msg.Type == tea.KeyPgUp {
			m.scroll = max(0, m.scroll-max(1, m.height-3))
			return m, nil
		}
		if msg.Type == tea.KeyEsc {
			m.page = "main"
			m.status = ""
			m.confirm = false
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
				m.cursor = min(4, m.cursor+1)
				m.scroll = 0
			case "up", "k":
				m.cursor = max(0, m.cursor-1)
				m.scroll = 0
			case "r":
				if !m.busy {
					m.operation++
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
					m.scroll = 0
				default:
					return m.inspect()
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
		return "Esc back"
	}
	if m.cursor < 2 {
		return "↑↓ select · Enter copy · r refresh · PgUp/PgDn scroll"
	}
	return "↑↓ select · Enter inspect · r refresh · PgUp/PgDn scroll"
}
