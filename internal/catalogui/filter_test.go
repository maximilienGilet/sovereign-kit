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
