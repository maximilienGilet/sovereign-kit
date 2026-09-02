package catalogui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCatalogReservesSearchRowAndFitsTheTerminalHeight(t *testing.T) {
	model := New(DefaultEntries())
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	view := updated.(Model).View()
	if !strings.Contains(view, "Filter:") {
		t.Fatalf("catalog does not reserve the bubbles search row:\n%s", view)
	}
	if lines := strings.Count(view, "\n"); lines > 20 {
		t.Fatalf("catalog overflowed %d-line terminal with %d lines:\n%s", 20, lines, view)
	}
}
