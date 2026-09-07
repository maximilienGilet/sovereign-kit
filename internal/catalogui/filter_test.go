package catalogui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCatalogDelegatesFilteringToCharmBubblesList(t *testing.T) {
	model := New(DefaultEntries())
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	view := updated.(Model).View()
	if !strings.Contains(view, "Filter:") {
		t.Fatalf("catalog does not expose the bubbles list filter:\n%s", view)
	}
}

func TestCatalogFilterKeepsEnterAndSpaceForFiltering(t *testing.T) {
	model := New(DefaultEntries())
	messages := []tea.Msg{
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Qwen")},
		tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Studio")},
		tea.KeyMsg{Type: tea.KeyEnter},
	}
	for _, message := range messages {
		updated, _ := model.Update(message)
		model = updated.(Model)
	}
	if value := model.list.FilterValue(); value != "Qwen Studio" {
		t.Fatalf("filter value = %q, want %q", value, "Qwen Studio")
	}
	if model.list.SettingFilter() {
		t.Fatal("enter did not apply the filter")
	}
}
