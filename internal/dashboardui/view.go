package dashboardui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/clientprofile"
	"github.com/maximilienGilet/sovereign-kit/internal/terminalui"
)

var (
	accent      = lipgloss.NewStyle().Foreground(lipgloss.Color("80"))
	muted       = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	bright      = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	borderColor = lipgloss.Color("238")
)

// Only data is sanitized; trusted layout styles remain intact.
func clean(value string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r >= 32 && r != 127 && (r < 128 || r > 159) {
			return r
		}
		return -1
	}, ansi.Strip(value))
}

func card(title, body string, width int) string {
	inner := max(1, width-4)
	body = ansi.Hardwrap(body, inner, true)
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(borderColor).
		Padding(0, 1).Width(max(3, width-2)).Render(accent.Bold(true).Render(ansi.Truncate(title, inner, "")) + "\n" + body)
}

func choice(selected bool, value string) string {
	if selected {
		return accent.Render("› " + clean(value))
	}
	return bright.Render("  " + clean(value))
}

func (m Model) endpointStatus() string {
	if !m.healthy {
		return "Tunnel disconnected"
	}
	if m.endpoint.Problem != "" {
		return clean(m.endpoint.Problem)
	}
	if m.endpoint.ID == "" {
		return "Model discovery pending"
	}
	return "Model verified · SSH tunnel open"
}

func (m Model) endpointActionsBody() string {
	model := m.endpoint.ID
	if model == "" {
		model = "Not verified"
	}
	lines := []string{
		choice(m.cursor == 0, "Base URL: "+m.endpoint.BaseURL),
		choice(m.cursor == 1, "Model: "+model),
		choice(m.cursor == 2, "Connect an OpenAI-compatible app"),
		muted.Render("  Optional integrations"),
	}
	if m.width >= 60 && m.width < 96 {
		return strings.Join(append(lines, choice(m.cursor == 3, "Pi")+"  "+choice(m.cursor == 4, "Oh My Pi (OMP)")+"  "+choice(m.cursor == 5, "OpenCode")), "\n")
	}
	return strings.Join(append(lines,
		choice(m.cursor == 3, "Pi"),
		choice(m.cursor == 4, "Oh My Pi (OMP)"),
		choice(m.cursor == 5, "OpenCode"),
	), "\n")
}

func measure(value *float64, unit string) string {
	if value == nil {
		return "not reported"
	}
	return fmt.Sprintf("%g%s", *value, unit)
}

func ratio(value *float64) string {
	if value == nil {
		return "not reported"
	}
	return fmt.Sprintf("%.1f%%", *value*100)
}

func metricLine(label, value string) string {
	return fmt.Sprintf("%-20s%s", label, value)
}

func (m Model) statusStripBody() string {
	items := []string{
		"Model  " + bright.Render(m.endpointStatus()),
		"Active requests  " + measure(m.stats.Active, ""),
		"Queued requests  " + measure(m.stats.Queued, ""),
	}
	if m.width >= 60 {
		body := strings.Join(items, "   ")
		if activity := m.activityState(); activity != "" {
			body += "\n" + muted.Render(activity)
		}
		return body
	}
	if activity := m.activityState(); activity != "" {
		items = append(items, activity)
	}
	return strings.Join(items, "\n")
}

func (m Model) activityState() string {
	if !m.healthy || m.stats.Problem != "" {
		return "Telemetry interrupted"
	}
	if m.stats.Active == nil || m.stats.Queued == nil {
		return "Waiting for telemetry"
	}
	if *m.stats.Active == 0 && *m.stats.Queued == 0 {
		return "Idle"
	}
	return ""
}

func (m Model) throughputReferenceTime() time.Time {
	return time.Now()
}

func (m Model) throughputPanelBody(width int) string {
	current := m.stats.DecodeTokensPerSecond
	if m.stats.Problem != "" || !m.healthy {
		current = nil
	}
	if width < 48 {
		lines := []string{"Current", measure(current, " tok/s")}
		if m.stats.Problem != "" {
			lines = append([]string{"Telemetry interrupted"}, lines...)
		}
		return strings.Join(lines, "\n")
	}
	lines := []string{metricLine("Current generation", measure(current, " tok/s"))}
	now := m.throughputReferenceTime()
	chartHeight := 6
	if m.width >= 120 {
		// Reserve the status strip, lower cards, heading/footer and chart label.
		// The remaining room belongs to the ten-minute plot, up to 14 rows.
		chartHeight = min(14, max(6, m.height-26))
	}
	chart := renderThroughputChart(m.throughput.snapshot(now), now, max(1, width-4), chartHeight, m.stats.Problem != "" || !m.healthy)
	if chart != "" {
		lines = append(lines, chart)
	}
	return strings.Join(lines, "\n")
}

func (m Model) inspectorBody(width int) string {
	contextLimit, outputLimit := "not reported", "not reported"
	if m.endpoint.Installable() {
		contextLimit = fmt.Sprintf("%d tokens", m.endpoint.ContextWindow)
		outputLimit = fmt.Sprintf("%d tokens", m.endpoint.MaxTokens)
	}
	summary := m.throughput.summary(m.throughputReferenceTime())
	if width < 36 {
		return strings.Join([]string{
			"Prompt", measure(m.stats.PromptTokensPerSecond, " tok/s"),
			"KV", ratio(m.stats.KVUsageRatio),
			"Context", contextLimit,
			"Output", outputLimit,
			"Average", measure(summary.Average, " tok/s"),
			"Peak", measure(summary.Peak, " tok/s"),
		}, "\n")
	}
	lines := []string{
		metricLine("Prompt throughput", measure(m.stats.PromptTokensPerSecond, " tok/s")),
		metricLine("KV cache usage", ratio(m.stats.KVUsageRatio)),
		metricLine("Context window", contextLimit),
		metricLine("Output limit", outputLimit),
		metricLine("Generation average", measure(summary.Average, " tok/s")),
		metricLine("Generation peak", measure(summary.Peak, " tok/s")),
	}
	if m.stats.KVUsageRatio != nil {
		bar := progress.New(progress.WithWidth(max(8, width-4)), progress.WithSolidFill("#60DBC0"), progress.WithoutPercentage())
		lines = append(lines, bar.ViewAs(*m.stats.KVUsageRatio))
	}
	if !m.stats.CheckedAt.IsZero() {
		lines = append(lines, muted.Render("Server report · "+m.stats.CheckedAt.Format("15:04:05")))
	}
	if m.endpoint.LimitsProblem != "" {
		lines = append(lines, clean(m.endpoint.LimitsProblem))
	}
	return strings.Join(lines, "\n")
}

func (m Model) sessionBody() string {
	return m.instanceBody() + "\n\n" + m.sessionActivityBody()
}

func (m Model) sessionActivityBody() string {
	if !m.logsExpanded {
		if m.serverLogs != "" {
			return muted.Render("Server logs · folded · l expand")
		}
		if len(m.events) > 0 {
			return "Session events · l expand\n" + muted.Render(m.events[len(m.events)-1])
		}
		return muted.Render("Session events · none yet · l expand")
	}
	events := muted.Render("No session events yet")
	logTitle := "Session events"
	if len(m.events) > 0 {
		events = muted.Render(strings.Join(m.events, "\n"))
	}
	if m.serverLogs != "" {
		logTitle = "Server logs · last snapshot"
		if !m.serverLogsAt.IsZero() {
			logTitle += " " + m.serverLogsAt.Format("15:04")
		}
		logLines := strings.Split(strings.TrimSpace(clean(m.serverLogs)), "\n")
		events = muted.Render(strings.Join(logLines[max(0, len(logLines)-4):], "\n"))
	}
	if m.width < 96 {
		if len(m.events) > 0 && m.serverLogs == "" {
			events = muted.Render(m.events[len(m.events)-1])
		}
		if m.serverLogs != "" {
			lines := strings.Split(events, "\n")
			events = lines[len(lines)-1]
		}
	}
	return logTitle + "\n" + events
}

func (m Model) mainView() string {
	if m.width >= 60 && m.width < 120 && m.height <= 26 {
		return m.compactMainView()
	}
	status := card("Status", m.statusStripBody(), m.width)
	var performance string
	if m.width >= 120 {
		inspectorWidth := max(38, (m.width-1)/3)
		chartWidth := m.width - inspectorWidth - 1
		performance = lipgloss.JoinHorizontal(lipgloss.Top,
			card("Generation throughput · live 10 minutes", m.throughputPanelBody(chartWidth), chartWidth), " ",
			card("Inspector", m.inspectorBody(inspectorWidth), inspectorWidth))
	} else {
		performance = lipgloss.JoinVertical(lipgloss.Left,
			card("Generation throughput · live 10 minutes", m.throughputPanelBody(m.width), m.width),
			card("Inspector", m.inspectorBody(m.width), m.width))
	}
	lower := lipgloss.JoinVertical(lipgloss.Left,
		card("Session & instance", m.sessionBody(), m.width),
		card("Endpoint & integrations", m.endpointActionsBody(), m.width))
	if m.width >= 120 {
		sessionWidth := (m.width - 1) / 2
		lower = lipgloss.JoinHorizontal(lipgloss.Top,
			card("Session & instance", m.sessionBody(), sessionWidth), " ",
			card("Endpoint & integrations", m.endpointActionsBody(), m.width-sessionWidth-1))
	}
	return lipgloss.JoinVertical(lipgloss.Left, status, performance, lower)
}

// A short terminal keeps connection facts and live activity together; secondary
// measurements and session details remain available by scrolling.
func (m Model) compactMainView() string {
	instance := strings.Split(m.instanceBody(), "\n")
	status := m.endpointStatus()
	if activity := m.activityState(); activity != "" {
		status += " · " + activity
	}
	current := m.stats.DecodeTokensPerSecond
	if m.stats.Problem != "" || !m.healthy {
		current = nil
	}
	now := m.throughputReferenceTime()
	summary := m.throughput.summary(now)
	contextLimit, outputLimit := "not reported", "not reported"
	if m.endpoint.Installable() {
		contextLimit = fmt.Sprintf("%d tokens", m.endpoint.ContextWindow)
		outputLimit = fmt.Sprintf("%d tokens", m.endpoint.MaxTokens)
	}
	lines := []string{
		accent.Bold(true).Render("Status") + " · " + status,
		"Active requests  " + measure(m.stats.Active, "") + "   Queued requests  " + measure(m.stats.Queued, ""),
		accent.Bold(true).Render("Generation throughput · live 10 minutes"),
		metricLine("Current generation", measure(current, " tok/s")) + " · Average " + measure(summary.Average, " tok/s") + " · Peak " + measure(summary.Peak, " tok/s"),
		renderThroughputChart(m.throughput.snapshot(now), now, m.width, 4, m.stats.Problem != "" || !m.healthy),
		accent.Render("Inspector") + " · Prompt " + measure(m.stats.PromptTokensPerSecond, " tok/s") + " · KV " + ratio(m.stats.KVUsageRatio),
		"Context " + contextLimit + " · Output " + outputLimit,
		accent.Render("Session & instance") + " · " + instance[0],
		accent.Bold(true).Render("Endpoint & integrations"),
		m.endpointActionsBody(),
	}
	if len(instance) > 1 {
		lines = append(lines, strings.Join(instance[1:], "\n"))
	}
	lines = append(lines, m.sessionActivityBody())
	if !m.stats.CheckedAt.IsZero() {
		lines = append(lines, muted.Render("Server report · "+m.stats.CheckedAt.Format("15:04:05")))
	}
	if m.endpoint.LimitsProblem != "" {
		lines = append(lines, clean(m.endpoint.LimitsProblem))
	}
	return ansi.Hardwrap(strings.Join(lines, "\n"), m.width, true)
}

func (m Model) instanceBody() string {
	identity := "Instance ID not supplied"
	if m.instance > 0 {
		identity = fmt.Sprintf("#%d", m.instance)
	}
	if m.session.Provider != "" {
		identity = clean(m.session.Provider) + " · " + identity
	}
	lines := []string{identity}
	if m.session.GPU != "" {
		lines = append(lines, "GPU       "+clean(m.session.GPU))
	}
	if m.session.Region != "" {
		lines = append(lines, "Region    "+clean(m.session.Region))
	}
	if m.session.HourlyPrice != nil {
		lines = append(lines, fmt.Sprintf("Rate      $%.3f / hour", *m.session.HourlyPrice))
	}
	if !m.session.StartedAt.IsZero() {
		lines = append(lines, "Connected "+m.session.StartedAt.Format("15:04 MST"))
	}
	return strings.Join(lines, "\n")
}

func (m Model) detailView() string {
	title := "Integration"
	lines := []string{}
	target := clean(fmt.Sprintf("%s\nDestination: %s", m.target.Kind, m.target.Path))
	switch m.page {
	case "generic":
		title = "Connect any OpenAI-compatible application"
		lines = append(lines,
			choice(m.genericCursor == 0, "Base URL: "+m.endpoint.BaseURL),
			choice(m.genericCursor == 1, "Model: "+m.endpoint.ID),
			choice(m.genericCursor == 2, "API key (if required): local-qwen-tunnel"), "",
			"Use these values in your application's provider settings.",
			"The loopback endpoint is keyless and protected by the SSH route.",
			"The placeholder above is not your Vast API key.", "",
			"This is an API base URL, not a browser page or OpenAPI specification.")
	case "inspecting":
		title = "Inspecting integration"
		lines = append(lines, "Checking the local profile and command (read-only)…", "", "Nothing is installed or changed during inspection.")
	case "confirm":
		title = "Review integration changes"
		lines = append(lines, target, "", "Inspection: "+clean(string(m.inspection.State)), clean(m.inspection.Detail), "", "Affected paths (existing managed files receive a recoverable backup):")
		for _, path := range m.inspection.Paths {
			lines = append(lines, clean(path))
		}
		lines = append(lines, "")
		if m.target.Kind == clientprofile.OpenCode {
			lines = append(lines, "This dedicated OpenCode config selects the Sovereign model and provider; the global config is not modified.")
		} else {
			lines = append(lines, "Existing settings, default models, skills, extensions and credentials are preserved.")
		}
		lines = append(lines, "No packages or extensions are installed.", "", choice(!m.confirm, "Cancel"), choice(m.confirm, "Install / update"))
	case "installing":
		title = "Updating integration"
		lines = append(lines, target, "", "Saving and verifying custom provider…", "Keep this connection open. Disconnect cancels the update.")
	case "ready":
		title = "Integration ready"
		lines = append(lines, "Ready — custom provider and CLI verified.", target, "", "In another terminal, from your project directory:", choice(true, m.inspection.Command), "", "Keep this connection open while using the application.")
		if m.inspection.Backup != "" {
			lines = append(lines, "Backup: "+clean(m.inspection.Backup))
		}
	case "error":
		title = "Integration setup failed"
		lines = append(lines, "No launch command is unlocked.", "", clean(m.status), "", "Retry inspection with r, or return to the endpoint with Esc.")
	case "blocked":
		title = "Integration unavailable"
		lines = append(lines, target, "Inspection: "+clean(string(m.inspection.State)), "", clean(m.status), "", "Return to the endpoint and refresh model discovery with r.")
	}
	if m.page == "confirm" && m.width >= 72 && m.height >= 30 {
		return accent.Bold(true).Render(title) + "\n\n" + strings.Join(lines, "\n")
	}
	return card(title, strings.Join(lines, "\n"), m.width)
}

func (m Model) View() string {
	terminalWidth := m.width
	// View has a value receiver: constrain this render without losing the real
	// terminal dimensions used by updates and future resize events.
	m.width = min(m.width, 140)
	body := m.detailView()
	if m.page == "main" {
		body = m.mainView()
	}
	if m.page == "confirm" && m.width >= 72 && m.height >= 30 {
		modal := terminalui.Modal(m.mainView(), body, m.width, m.height-2, true)
		if strings.Contains(modal, "╭") {
			body = modal
		}
	}
	wrapped := strings.Split(body, "\n")
	available := max(1, m.height-2)
	offset := min(m.scroll, max(0, len(wrapped)-available))
	if !m.manualScroll && m.scroll == 0 && (m.page == "main" && (m.cursor > 0 || m.mainNavigated) || m.page == "generic" && m.genericCursor > 0 || m.page == "confirm") {
		for index, line := range wrapped {
			if strings.Contains(ansi.Strip(line), "› ") && index >= available {
				offset = min(max(0, index-available+2), max(0, len(wrapped)-available))
				break
			}
		}
	}
	body = strings.Join(wrapped[offset:min(len(wrapped), offset+available)], "\n")
	heading := accent.Bold(true).Render("PRIVATE ENDPOINT")
	footer := "Tunnel not connected"
	if m.healthy {
		footer = "Tunnel open · keep this connection open"
	}
	if len(wrapped) > available {
		footer = "PgUp/PgDn scroll · " + footer
	}
	if m.status != "" && m.page != "error" && m.page != "blocked" {
		footer = clean(m.status)
	}
	view := ansi.Truncate(heading, m.width, "") + "\n" + body + "\n" + muted.Render(ansi.Truncate(footer, m.width, ""))
	if terminalWidth > m.width {
		padding := strings.Repeat(" ", (terminalWidth-m.width)/2)
		view = padding + strings.ReplaceAll(view, "\n", "\n"+padding)
	}
	if m.height >= 40 && len(wrapped) <= available {
		spare := m.height - (len(wrapped) + 2)
		view = strings.Repeat("\n", spare/2) + view + strings.Repeat("\n", spare-spare/2)
	}
	return view
}
