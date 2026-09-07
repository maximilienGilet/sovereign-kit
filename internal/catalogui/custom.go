package catalogui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const customWorkflow = "Search or enter owner/model.\nInspect compatibility.\nSet hardware needs before searching offers."

// Custom discovery has no recipe contract yet, so it never renders metrics.
func customWelcome(entry Entry, width, height int) string {
	yellow := lipgloss.NewStyle().Foreground(lipgloss.Color("#F2C879")).Bold(true)
	name := title.Render(ansi.Wrap(strings.Join(strings.Fields(entry.Name), " "), width, ""))
	action := yellow.Render("[ " + actionLabel(entry) + " ]")
	heading := yellow.Render("CHOOSE A MODEL")
	art := "     .-\"\"\"\"\"-.\n    /  ^   ^  \\\n   |    \\_/    |\n    \\         /\n  _.-'-------'-._\n (___/       \\___)"
	intro := label.Render(ansi.Wrap(customWorkflow, width, ""))
	parts := []string{name, heading, "", yellow.Render(art), "", intro, "", action}
	if lipgloss.Height(strings.Join(parts, "\n")) > height {
		parts = []string{name, heading, yellow.Render("\\( ^ v ^ )/"), intro, action}
	}
	if lipgloss.Height(strings.Join(parts, "\n")) > height || width < 40 {
		parts = []string{name, heading, intro, action}
	}
	return strings.Join(parts, "\n")
}
