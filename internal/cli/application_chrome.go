package cli

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Shared terminal chrome. Child models keep ownership of focus and scrolling.
func (m *applicationModel) ink(value, color string) string {
	if m.deps.Setup.Getenv("NO_COLOR") != "" {
		return value
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(value)
}

func (m *applicationModel) primaryPanel(heading, body, action string) string {
	width := min(76, m.width-4)
	if m.height < 18 || width < 36 {
		return heading + "\n\n" + body + "\n\n" + action
	}
	content := m.ink(heading, "#60DBC0") + "\n\n" + ansi.Wrap(body, width-6, "")
	if action != "" {
		content += "\n\n" + m.ink(action, "#60DBC0")
	}
	border := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Width(width - 2)
	if m.deps.Setup.Getenv("NO_COLOR") == "" {
		border = border.BorderForeground(lipgloss.Color("#3C6473"))
	}
	return m.centerPanel(border.Render(content))
}

func (m *applicationModel) centerPanel(body string) string {
	width := lipgloss.Width(body)
	left := strings.Repeat(" ", max(0, (m.width-width)/2))
	lines := strings.Split(body, "\n")
	for i := range lines {
		lines[i] = left + lines[i]
	}
	top := min(3, max(0, (m.height-6-len(lines))/3))
	return strings.Repeat("\n", top) + strings.Join(lines, "\n")
}

func (m *applicationModel) homeView() string {
	if m.configured {
		return m.primaryPanel("YOUR PRIVATE ROUTE", "Route configured, not verified.\n\nConnect to check the endpoint.\nReconfigure only with replacement approval."+m.homeError(), "[ Enter ] Connect     [ r ] Setup")
	}
	return m.primaryPanel("PRIVATE AI, YOUR WAY", "Configure your private route.\n\nRent a Vast GPU or connect an existing SSH host.\nUse the same private endpoint in any compatible application."+m.homeError(), "[ Enter ] Set up your server")
}

func (m *applicationModel) homeError() string {
	if m.errText == "" {
		return ""
	}
	return "\n\nConfiguration error: " + m.errText
}
