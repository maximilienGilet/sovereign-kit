package dashboardui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) View() string {
	lines := []string{}
	target := fmt.Sprintf("%s\nDestination: %s", m.target.Kind, m.target.Path)
	switch m.page {
	case "main":
		lines = append(lines, "Connection")
		values := []string{"Base URL: " + m.endpoint.BaseURL, "Model: " + m.endpoint.ID, "Connect any OpenAI-compatible application", "Pi / Oh My Pi", "OpenCode"}
		for i, value := range values {
			if i == 2 {
				lines = append(lines, "", "Connect an application")
			}
			if i == 3 {
				lines = append(lines, "Optional integrations")
			}
			prefix := "  "
			if i == m.cursor {
				prefix = "› "
			}
			lines = append(lines, prefix+value)
		}
		status := "Endpoint: model discovery pending"
		if m.endpoint.Problem != "" {
			status = "Endpoint: " + m.endpoint.Problem
		} else if m.endpoint.ID != "" {
			status = "Endpoint responding · model verified"
		}
		lines = append(lines, "", status)
		if m.endpoint.LimitsProblem != "" {
			lines = append(lines, m.endpoint.LimitsProblem)
		}
	case "generic":
		lines = append(lines, "Connect an OpenAI-compatible application", "Base URL: "+m.endpoint.BaseURL, "Model: "+m.endpoint.ID, "API key (if required): local-qwen-tunnel", "The loopback endpoint is keyless and protected by the SSH route. This placeholder is not your Vast API key.", "This is an API base URL, not a browser page or OpenAPI specification.")
	case "inspecting":
		lines = append(lines, "Inspecting integration profile (read-only)…")
	case "confirm":
		lines = append(lines, target, "Inspection: "+string(m.inspection.State), m.inspection.Detail, "Affected paths (existing managed files receive a recoverable backup):")
		lines = append(lines, m.inspection.Paths...)
		lines = append(lines, "Unrelated credentials, sessions and settings are preserved.", "Install/update only this integration? Packages may be downloaded.")
		if m.confirm {
			lines = append(lines, "  Cancel    › Install / update")
		} else {
			lines = append(lines, "› Cancel      Install / update")
		}
	case "installing":
		lines = append(lines, target, "Installing and verifying profile…", "Keep this connection open. Disconnect cancels the installer.")
	case "ready":
		lines = append(lines, target, "Ready — configuration and dependencies verified.", "In another terminal, from your project directory:", m.inspection.Command, "Keep this connection open while using the application.")
		if m.inspection.Backup != "" {
			lines = append(lines, "Backup: "+m.inspection.Backup)
		}
	case "error":
		lines = append(lines, "Integration setup failed. No launch command is unlocked.")
	case "blocked":
		lines = append(lines, "Profile installation unavailable")
		lines = append(lines, target, "Inspection: "+string(m.inspection.State))
	}
	if m.status != "" {
		lines = append(lines, "", m.status)
	}
	body := ansi.Strip(strings.Join(lines, "\n"))
	body = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r >= 32 && r != 127 {
			return r
		}
		return -1
	}, body)
	body = lipgloss.NewStyle().Width(max(1, m.width)).Render(body)
	wrapped := strings.Split(body, "\n")
	available := max(1, m.height-2)
	offset := min(m.scroll, max(0, len(wrapped)-available))
	body = strings.Join(wrapped[offset:min(len(wrapped), offset+available)], "\n")
	heading := "PRIVATE ENDPOINT"
	if m.instance > 0 {
		heading += fmt.Sprintf(" · Vast #%d", m.instance)
	}
	footer := "Tunnel not connected"
	if m.healthy {
		footer = "Tunnel open · reachable from this Mac while this connection stays open"
	}
	if len(wrapped) > available {
		footer = "PgUp/PgDn scroll · " + footer
	}
	return ansi.Truncate(heading, m.width, "") + "\n" + body + "\n" + ansi.Truncate(footer, m.width, "")
}
