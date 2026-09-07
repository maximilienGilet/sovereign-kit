package catalogui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestDashboardPlacesEachConfiguredMetricOnce(t *testing.T) {
	entry := pickerFixture()[0]
	for _, width := range []int{40, 88, 120, 249} {
		model := NewPicker([]Entry{entry})
		body := ansi.Strip(model.recipeDashboard(entry, width, true))
		for _, value := range []string{"32,768", "4,096", "1 request"} {
			if count := strings.Count(body, value); count != 1 {
				t.Errorf("width %d: %q appears %d times in dashboard", width, value, count)
			}
		}
	}
}

func TestCustomWelcomeHasNoPretendMetricsAndKeepsAction(t *testing.T) {
	entry := CustomEntry()
	for _, size := range [][2]int{{30, 10}, {72, 24}, {134, 30}, {150, 34}, {320, 80}} {
		model := NewPicker([]Entry{entry})
		updated, _ := model.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := ansi.Strip(updated.(Model).View())
		for _, bad := range []string{"UNKNOWN", "unknown", "CONTEXT", "OUTPUT", "CAPACITY", "VRAM", "THROUGHPUT", "not live usage"} {
			if strings.Contains(view, bad) {
				t.Errorf("%dx%d custom view contains %q", size[0], size[1], bad)
			}
		}
		for _, want := range []string{"Custom Hugging Face", "INSPECT HUGGING FACE", "choose"} {
			if !strings.Contains(view, want) {
				t.Errorf("%dx%d missing %q", size[0], size[1], want)
			}
		}
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatal("custom view overflows")
		}
	}
}

func TestCustomWelcomeExplainsWorkflowAtEverySize(t *testing.T) {
	for _, size := range [][2]int{{30, 10}, {72, 24}, {134, 30}, {150, 34}, {320, 80}} {
		model := NewPicker([]Entry{CustomEntry()})
		updated, _ := model.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		model = updated.(Model)
		text := strings.Join(strings.Fields(strings.ReplaceAll(ansi.Strip(model.View()), "│", " ")), " ")
		for _, want := range []string{"CHOOSE A MODEL", "Search or enter owner/model.", "Inspect compatibility.", "Set hardware needs before", "searching offers."} {
			if !strings.Contains(text, want) {
				t.Errorf("%dx%d missing workflow %q", size[0], size[1], want)
			}
		}
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
		model = updated.(Model)
		var content strings.Builder
		for {
			content.WriteString(ansi.Strip(model.evidence.View()))
			content.WriteByte(' ')
			if model.evidence.AtBottom() {
				break
			}
			updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
			model = updated.(Model)
		}
		text = strings.Join(strings.Fields(content.String()), " ")
		for _, want := range []string{"owner/", "model", "compatibility", "hardware", "offers", "does not guarantee"} {
			if !strings.Contains(text, want) {
				t.Errorf("%dx%d details missing %q", size[0], size[1], want)
			}
		}
	}
}
