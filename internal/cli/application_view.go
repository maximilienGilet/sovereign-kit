package cli

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/catalogui"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
)

func (m *applicationModel) resizeChild() {
	width, height := max(1, m.width), max(1, m.height-4)
	if warning := m.paidPromptWarning(); warning != "" {
		height = max(1, height-len(strings.Split(ansi.Wrap(warning, width, ""), "\n")))
	}
	m.recipeViewport.Width = width
	m.recipeViewport.Height = height
	m.diagnostic.Width = width
	m.diagnostic.Height = height
	m.confirmation.Width = width
	m.confirmation.Height = max(1, height-3)
	if m.confirmText != "" {
		if width >= 48 && height >= 18 {
			width = min(62, width-10)
			m.confirmation.Width = width
			m.confirmation.Height = min(10, height-10)
		}
		m.confirmation.SetContent(ansi.Wrap(m.sanitize(m.confirmText), width, ""))
		height = 3
	}
	if form, ok := m.child.(*huh.Form); ok {
		if m.resizeFormDescription != nil {
			m.resizeFormDescription(height < 12)
		}
		form.WithWidth(min(76, width)).WithHeight(height)
	}
	childHeight := height
	childWidth := width
	if m.pending != nil && m.pending.kind == promptCost && m.width >= 80 && m.height >= 30 {
		childWidth, childHeight = 62, min(18, height-6)
	}
	if _, ok := m.child.(*huh.Form); ok {
		childWidth = min(76, width)
	}
	if picker, ok := m.child.(catalogui.Model); ok {
		childHeight = height + 2
		if !picker.EvidenceOpen() {
			childHeight = max(12, childHeight)
		}
	}
	if m.child != nil {
		m.child, _ = m.child.Update(tea.WindowSizeMsg{Width: childWidth, Height: childHeight})
	}
	m.dashboard, _ = updateDashboardSize(m.dashboard, width, max(1, m.height-4))
}
func (m *applicationModel) View() string {
	if m.width < 30 || m.height < 10 {
		if m.exitConfirm {
			return fitApplication(m.exitView(), m.width, m.height)
		}
		if m.screen == "dashboard" {
			return fitApplication(m.dashboard.View()+"\nEnter · PgUp/Dn · d/q", m.width, m.height)
		}
		return fitApplication("SOVEREIGN KIT\nEnlarge to at least 30×10.\nCtrl+C to stop.", m.width, m.height)
	}
	title := "SOVEREIGN KIT / " + strings.ToUpper(m.screen)
	if m.destruction != nil && m.destruction.action == "stop" {
		title = "SOVEREIGN KIT / STOPPING"
	}
	if m.logsExpanded && !m.exitConfirm {
		return fitApplication(m.expandedServerLogs(), m.width, m.height)
	}
	if m.pending != nil {
		title = "SOVEREIGN KIT / " + string(m.pending.kind)
	}
	body, footer := "", ""
	switch m.screen {
	case "resume":
		body = fmt.Sprintf("Unfinished deployment\n%s\n\n%s\n\nContinue the existing instance. No new instance will be created.", m.status, m.compactPaidWarning())
		if !m.creating && m.instanceID == 0 {
			body = "Recover an existing Vast instance\n\nNo local checkpoint was found. Enter the instance ID shown in Vast, then confirm its recipe and SSH identity.\n\nNo new instance will be created."
		} else if m.instanceID <= 0 {
			body += "\nCreation outcome uncertain: check Vast, then use sovkit resume INSTANCE_ID."
		}
		footer = "Enter resume · d destroy · q quit"
		if m.width < 40 {
			footer = "Enter resume / d delete / q"
		}
	case "home":
		if m.configured {
			footer = "Enter connect · r setup · q quit"
		} else {
			footer = "Enter setup · q quit"
		}
		body = m.homeView()
	case "replace":
		body = "A configuration already exists.\nReplace it?\n\nNo setup or paid creation starts without approval."
		choice := "NO"
		if m.selected {
			choice = "YES"
		}
		footer = "←→ " + choice + " · Enter · Esc cancel"
	case "prompt":
		if m.child != nil {
			body = m.child.View()
		}
		if picker, ok := m.child.(catalogui.Model); ok {
			lines := strings.Split(strings.TrimSuffix(body, "\n"), "\n")
			if len(lines) > 0 && strings.Contains(ansi.Strip(lines[0]), "SOVEREIGN KIT") {
				lines = lines[1:]
			}
			if len(lines) > 0 {
				lines = lines[:len(lines)-1]
			}
			body = strings.Join(lines, "\n")
			if !picker.EvidenceOpen() {
				m.recipeViewport.SetContent(body)
				body = m.recipeViewport.View()
			}
		}
		if m.confirmText != "" {
			body = m.confirmation.View() + "\n" + body
			footer = "PgUp/PgDn read · ←→ · Enter"
		} else {
			footer = "Enter next · Esc back"
		}
		if m.locked {
			footer = "Enter submit · Ctrl+C stop"
			if m.confirmText != "" {
				footer = "←→ Enter · PgDn · Ctrl+C"
			}
		}
		if m.pending != nil && m.pending.kind == promptCost {
			if m.locked {
				footer = "←→ Enter · Ctrl+C stop · PgDn"
			} else {
				footer = "←→ · Enter · Esc back · PgDn"
			}
		}
		if m.pending != nil && m.pending.kind == promptWorkload {
			footer = "Enter · i info · Esc · PgDn"
		}
		if browser, ok := m.child.(*offerBrowserModel); ok {
			footer = browser.Footer()
			left := strings.Repeat(" ", max(0, (m.width-browser.width)/2))
			lines := strings.Split(body, "\n")
			for i := range lines {
				lines[i] = left + lines[i]
			}
			body = strings.Join(lines, "\n")
		}
		if warning := m.paidPromptWarning(); warning != "" {
			body = warning + "\n" + body
		}
		if _, ok := m.child.(*huh.Form); ok && m.confirmText == "" && m.width >= 90 {
			body = m.centerPanel(body)
		}
	case "working", "connecting", "destroying":
		reserve := 0
		if m.creating || m.instanceID > 0 {
			reserve = len(strings.Split(ansi.Wrap(m.compactPaidWarning(), m.width, ""), "\n"))
		}
		available := m.height - 4 - reserve
		logRows := 0
		if m.screen == "working" && m.progressStage == setup.ProgressLaunching {
			logRows = min(7, max(2, available/3))
			available -= logRows + 1
		}
		body = m.loader.view(m.width, available, m.deps.Setup.Getenv("NO_COLOR") == "", "")
		if logRows > 0 {
			body += "\n\n" + m.compactServerLogs(min(m.width, 100), logRows)
		}
		footer = "Ctrl+C stop"
		if m.screen == "connecting" && m.instanceID > 0 {
			footer = "Esc / Ctrl+C · stop, leave running or destroy"
		}
		if logRows > 0 {
			footer = "l logs · Ctrl+C stop"
		}
	case "saved":
		body = "Configuration saved.\nRoute not verified.\n\nConnect to verify the endpoint."
		footer = "Enter connect · Esc home · q quit"
	case "restart-confirm":
		choices := "> Cancel\n  Restart and connect"
		if m.selected {
			choices = "  Cancel\n> Restart and connect"
		}
		body = fmt.Sprintf("Restart instance #%d?\n\nThis instance is stopped.\nRestarting resumes GPU billing.\nThe saved model and data will be reused.\n\n%s", m.instanceID, choices)
		footer = "↑↓ choose · Enter confirm · Esc cancel"
	case "error":
		body = m.errText
		if m.creating || m.instanceID > 0 {
			body = m.loader.frozen(m.width, m.height, m.deps.Setup.Getenv("NO_COLOR") == "", "!") + body
		}
		if m.recoveryText != "" {
			body += "\n" + m.recoveryText
		}
		footer = "PgDn read · Esc back · q quit"
		if m.locked {
			footer = "PgDn read · q quit"
		}
		if m.canDestroy() {
			footer = "d destroy · PgDn · q quit"
		}
		if !m.serverLogs.CheckedAt.IsZero() {
			body += "\n\n" + m.compactServerLogs(m.width, 6)
			footer = "l logs · " + footer
		}
	case "destroy-confirm":
		body = m.destroyDialog(m.instanceID)
	case "destroyed":
		body = m.recoveryText
		footer = "q quit"
	case "dashboard":
		body = m.dashboard.View()
		footer = m.dashboard.ActionHint() + " · d disconnect · q quit"
		if m.connection != nil && !m.connection.healthy {
			footer = "Checking route · d disconnect"
		}
	}
	if m.locked && (m.screen == "working" || m.screen == "connecting" || m.screen == "saved" || m.screen == "destroying") {
		if m.screen != "saved" {
			body += "\n" + m.compactPaidWarning()
		} else {
			body = m.compactPaidWarning() + "\n" + body
		}
	}
	if !m.exitConfirm {
		if m.screen == "prompt" && m.pending != nil && m.pending.kind == promptCost && m.width >= 80 && m.height >= 30 {
			body = m.modal("", body)
		}
		if m.screen == "prompt" && m.confirmText != "" {
			body = m.modal(m.loader.view(m.width, m.height-4, false, ""), body+"\n"+footer)
			footer = ""
		}
		switch m.screen {
		case "replace", "destroy-confirm", "restart-confirm":
			body = m.modal(m.loader.view(m.width, m.height-4, false, ""), body+"\n\n"+footer)
			footer = ""
		case "resume", "saved", "destroyed":
			body = m.primaryPanel(strings.ToUpper(m.screen), body, "")
		}
	}
	if m.screen == "working" || m.screen == "connecting" || m.screen == "destroying" {
		// Keep identity and billing in the same bounded composition as the
		// loader, without moving the keyboard footer or clipping small screens.
		blockWidth := min(m.width, 100)
		body = ansi.Wrap(body, blockWidth, "")
		lines := strings.Split(body, "\n")
		left := strings.Repeat(" ", max(0, (m.width-blockWidth)/2))
		for i := range lines {
			lines[i] = left + lines[i]
		}
		top := max(0, (m.height-4-len(lines))/2)
		body = strings.Repeat("\n", top) + strings.Join(lines, "\n")
	}
	if m.exitConfirm {
		body = m.modal(body, m.exitView())
		footer = ""
	}
	// Redact before rendering, while preserving the UI's own ANSI styling.
	for _, secret := range m.secrets {
		body = strings.ReplaceAll(body, secret, "[redacted]")
	}
	if m.screen == "error" && !m.exitConfirm {
		prefix := ""
		if m.locked {
			prefix = ansi.Wrap(m.compactPaidWarning(), m.width, "") + "\n"
		}
		prefix += m.ink("! ACTION REQUIRED", "#FFB454") + "\n\n"
		// Paid identity remains pinned; only the diagnostic scrolls.
		reserved := 0
		if prefix != "" {
			reserved = len(strings.Split(strings.TrimSuffix(prefix, "\n"), "\n"))
		}
		m.diagnostic.Height = max(1, m.height-4-reserved)
		m.diagnostic.SetContent(ansi.Wrap(body, m.width, ""))
		body = prefix + m.diagnostic.View()
	}
	lines := strings.Split(ansi.Wrap(body, m.width, ""), "\n")
	height := m.height - 4
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	header := m.applicationHeader(title)
	return header + "\n" + m.ink(strings.Repeat("─", m.width), "#3C6473") + "\n" + strings.Join(lines, "\n") + "\n" + m.ink(ansi.Truncate(footer, m.width, ""), "#A7C1CB") + "\n"
}
func (m *applicationModel) paidPromptWarning() string {
	if m.screen == "prompt" && m.instanceID > 0 {
		return fmt.Sprintf("Instance #%d\nWarning: billing may apply.", m.instanceID)
	}
	if m.screen == "prompt" && m.creating {
		return "Creation uncertain.\nWarning: billing may apply."
	}
	return ""
}
func fitApplication(value string, width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}
	lines := strings.Split(value, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "")
	}
	return strings.Join(lines, "\n")
}
func progressLabel(stage string) string {
	switch stage {
	case setup.ProgressModelSearch:
		return "Searching Hugging Face models…"
	case setup.ProgressModelInspect:
		return "Inspecting Hugging Face model…"
	case setup.ProgressSearching:
		return "Searching Vast offers…"
	case setup.ProgressCreating:
		return "Creating paid instance — do not repeat…"
	case setup.ProgressCreated:
		return "Instance created. Billing may be active."
	case setup.ProgressWaiting:
		return "Waiting for the instance…"
	case setup.ProgressHostKeys:
		return "Securing SSH host…"
	case setup.ProgressLaunching:
		return "Launching inference server…"
	case setup.ProgressSaving:
		return "Saving configuration…"
	default:
		return "Working…"
	}
}
