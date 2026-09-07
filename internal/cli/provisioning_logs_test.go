package cli

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"strings"
	"testing"
	"time"
)

func TestServerLogsPanelExpansionAndErrorRetention(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	m.screen = "working"
	m.applyProgress(setup.Progress{Stage: setup.ProgressLaunching, InstanceID: 987})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.applyServerLogs(setup.ServerLogs{Text: "Downloading model\nCUDA failure", CheckedAt: time.Now()})
	if !strings.Contains(m.View(), "CUDA failure") {
		t.Fatal("compact logs hidden")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if !m.logsExpanded || !strings.Contains(m.View(), "SERVER LOGS") {
		t.Fatal("expanded logs unavailable")
	}
	m.screen = "error"
	m.errText = "server exited"
	if !strings.Contains(m.View(), "CUDA failure") {
		t.Fatal("error lost logs")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.logsExpanded || !strings.Contains(m.View(), "server exited") {
		t.Fatal("cannot return to diagnostic")
	}
}

func TestCompactLogsHideProgressProtocolButRetainRawSnapshot(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	defer m.cleanup()
	for _, raw := range []string{
		"Downloading weights\nSOVKIT_DOWNLOAD {\"current\": 50, \"total\": 100}",
		"SOVKIT_DOWNLOAD {\"current\": 50, \"total\": 100}",
	} {
		m.applyServerLogs(setup.ServerLogs{Text: raw, CheckedAt: time.Now()})
		view := m.compactServerLogs(100, 6)
		if strings.Contains(view, "SOVKIT_DOWNLOAD") || len(strings.Split(view, "\n")) < 2 {
			t.Fatalf("compact log protocol leaked or no feedback: %s", view)
		}
		if m.serverLogs.Text != raw || !strings.Contains(m.expandedServerLogs(), "SOVKIT_DOWNLOAD") {
			t.Fatal("raw logs were removed")
		}
	}
}
