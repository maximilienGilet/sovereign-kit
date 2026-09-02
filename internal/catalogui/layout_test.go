package catalogui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCatalogUsesACompactRecipeListInsteadOfAWideDataTable(t *testing.T) {
	model := New(DefaultEntries())
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 150, Height: 42})
	view := updated.(Model).View()
	if strings.Contains(view, "RECIPE                         KIND") {
		t.Fatalf("catalog still uses the wide table layout:\n%s", view)
	}
	for _, expected := range []string{"RECIPES", "Qwen Studio", "CUSTOM", "HARDWARE FIT", "PERFORMANCE"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("catalog view missing %q:\n%s", expected, view)
		}
	}
}
