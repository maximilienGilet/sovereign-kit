package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

// The selected instance is captured when the dialog opens, never inferred again
// from a newer route when the user confirms a destructive action.
func (m *applicationModel) exitKey(key tea.KeyMsg) tea.Cmd {
	if key.Type == tea.KeyEsc || key.String() == "n" {
		m.exitConfirm, m.exitDestroy = false, false
		return nil
	}
	if m.exitInstance <= 0 {
		if key.String() == "y" {
			return m.stopAndQuit()
		}
		return nil
	}
	if m.exitDestroy {
		switch key.String() {
		case "left", "right", "up", "down", "tab", " ":
			m.selected = !m.selected
		case "enter":
			if !m.selected {
				m.exitDestroy = false
				return nil
			}
			return m.confirmRemoteExit("destroy")
		}
		return nil
	}
	switch key.String() {
	case "down", "j", "right", "tab":
		m.exitChoice = (m.exitChoice + 1) % 4
	case "up", "k", "left":
		m.exitChoice = (m.exitChoice + 3) % 4
	case "enter":
		switch m.exitChoice {
		case 0:
			return m.confirmRemoteExit("stop")
		case 1:
			return m.stopAndQuit()
		case 2:
			m.exitDestroy = true
			m.selected = false
		case 3:
			m.exitConfirm = false
		}
	}
	return nil
}

func (m *applicationModel) exitView() string {
	if m.exitInstance <= 0 {
		return "Stop local work and exit?\ny exit · n/Esc continue\n" + m.paidWarning()
	}
	if m.exitDestroy {
		return m.destroyDialog(m.exitInstance)
	}
	labels := []string{"Stop instance and quit", "Leave running and quit", "Destroy instance…", "Cancel"}
	spacious := m.width >= 76 && m.height >= 30
	lines := []string{m.ink(fmt.Sprintf("Quit · Vast #%d", m.exitInstance), "#60DBC0")}
	if spacious {
		lines = append(lines, "", "Choose what happens to your remote instance.", "")
	}
	if m.exitError != "" {
		lines = append(lines, ansi.Truncate(strings.Join(strings.Fields(m.sanitize(m.exitError)), " "), min(60, max(1, m.width-6)), "…"))
	}
	for i, label := range labels {
		lines = append(lines, m.dialogChoice(label, i == m.exitChoice, i == 2))
		if spacious {
			lines = append(lines, "")
		}
	}
	consequences := []string{"Stops queued starts; storage billing may continue.", "Remote billing / queued starts remain active.", "Next: confirm permanent deletion.", "Keep this session open; billing is unchanged."}
	if m.height >= 12 {
		lines = append(lines, consequences[m.exitChoice%4])
	}
	if spacious {
		lines = append(lines, "")
	}
	return strings.Join(append(lines, "↑↓ choose · Enter confirm · Esc cancel"), "\n")
}

func (m *applicationModel) dialogChoice(label string, selected, danger bool) string {
	prefix := "  "
	if selected {
		prefix = "› "
	}
	value := prefix + label
	if m.width >= 76 && m.height >= 30 {
		value += strings.Repeat(" ", max(0, 60-ansi.StringWidth(value)))
	}
	if selected && m.deps.Setup.Getenv("NO_COLOR") == "" {
		tone := "#60DBC0"
		if danger {
			tone = "#FF7878"
		}
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#071217")).Background(lipgloss.Color(tone)).Bold(true).Render(value)
	}
	return value
}

func (m *applicationModel) destroyDialog(id int) string {
	if m.width < 60 || m.height < 18 {
		return fmt.Sprintf("Destroy #%d?\nDeletes remote instance data.\n%s\n%s\n↑↓ Enter · Esc back", id, m.dialogChoice("Cancel", !m.selected, false), m.dialogChoice("Permanently destroy", m.selected, true))
	}
	spacious := m.width >= 76 && m.height >= 30
	gap := "\n"
	if spacious {
		gap = "\n\n"
	}
	return m.ink(fmt.Sprintf("Destroy instance #%d?", id), "#FF7878") + gap +
		"Permanently deletes data on the remote instance.\nFiles on your Mac are not deleted." + gap +
		m.dialogChoice("Cancel", !m.selected, false) + gap +
		m.dialogChoice("Permanently destroy", m.selected, true) + gap +
		"↑↓ choose · Enter confirm · Esc back"
}

func (m *applicationModel) confirmRemoteExit(action string) tea.Cmd {
	if m.instanceID != m.exitInstance || m.exitInstance <= 0 {
		m.exitError = "Cannot proceed: instance changed. Cancel and reopen this dialog."
		return nil
	}
	capability := m.recovery
	if capability.InstanceID != m.exitInstance || (action == "stop" && capability.Stop == nil) || (action == "destroy" && capability.Destroy == nil) {
		var err error
		capability, err = m.savedRecovery(m.exitInstance)
		if err != nil {
			m.exitError = "Cannot manage instance: " + m.sanitize(err.Error())
			m.exitDestroy = false
			return nil
		}
	}
	if capability.InstanceID != m.exitInstance || (action == "stop" && capability.Stop == nil) || (action == "destroy" && capability.Destroy == nil) {
		m.exitError = "Cannot manage this instance: verified capability unavailable."
		m.exitDestroy = false
		return nil
	}
	m.recovery = capability
	m.exitConfirm, m.exitDestroy = false, false
	return m.beginInstanceOperation(action, true)
}

func (m *applicationModel) savedRecovery(id int) (setup.InstanceRecovery, error) {
	if m.deps.Recovery != nil {
		return m.deps.Recovery(m.ctx, id)
	}
	token := strings.TrimSpace(m.deps.Setup.Getenv("VAST_API_KEY"))
	if token == "" && m.deps.Setup.Credentials != nil {
		var err error
		token, err = m.deps.Setup.Credentials.Load()
		if err != nil {
			return setup.InstanceRecovery{}, fmt.Errorf("cannot read saved Vast API key; set VAST_API_KEY and retry")
		}
	}
	if token == "" {
		return setup.InstanceRecovery{}, fmt.Errorf("Vast API key unavailable; set VAST_API_KEY and retry")
	}
	m.rememberSecret(token)
	return savedInstanceRecovery(m.path, id, func(checkpoint string) setup.InstanceRecovery {
		return setup.NewInstanceRecovery(id, vast.NewClient("https://console.vast.ai", token), setup.Options{CheckpointPath: checkpoint, PollInterval: 2 * time.Second, PollTimeout: recoveryTimeout}, setup.RealClock{})
	})
}

// Resolve authority again after workers have joined. A newly created checkpoint
// must reach the constructor, so its provisioning lock cannot be bypassed.
func savedInstanceRecovery(path string, id int, build func(string) setup.InstanceRecovery) (setup.InstanceRecovery, error) {
	checkTarget := func() (string, error) {
		cp, err := setup.ReadCheckpoint(setup.CheckpointPath(path))
		if err == nil {
			if cp.InstanceID != id {
				return "", fmt.Errorf("pending deployment does not match instance #%d", id)
			}
			return setup.CheckpointPath(path), nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("pending deployment is unreadable")
		}
		cfg, err := config.Load(path)
		if err != nil || cfg.Provider.Kind != "vast" || cfg.Provider.InstanceID != id {
			return "", fmt.Errorf("saved route does not identify instance #%d", id)
		}
		return "", nil
	}
	if _, err := checkTarget(); err != nil {
		return setup.InstanceRecovery{}, err
	}
	wrap := func(action string) func(context.Context) error {
		return func(ctx context.Context) error {
			checkpoint, err := checkTarget()
			if err != nil {
				return err
			}
			capability := build(checkpoint)
			if capability.InstanceID != id {
				return fmt.Errorf("instance capability mismatch")
			}
			f := capability.Destroy
			if action == "stop" {
				f = capability.Stop
			}
			if f == nil {
				return fmt.Errorf("instance %s capability unavailable", action)
			}
			return f(ctx)
		}
	}
	return setup.InstanceRecovery{InstanceID: id, Stop: wrap("stop"), Destroy: wrap("destroy")}, nil
}
