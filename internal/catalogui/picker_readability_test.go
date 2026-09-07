package catalogui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// A status badge must not consume the recipe name in the navigation rail.
func TestPickerRailKeepsRecipeNamesReadable(t *testing.T) {
	for _, size := range []struct{ width, height int }{{80, 24}, {150, 40}} {
		model := NewPicker(pickerFixture())
		updated, _ := model.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
		model = updated.(Model)
		rail := ansi.Strip(model.pickerList(model.pickerSidebarWidth(), size.height-2))
		for _, name := range []string{"Qwen Solo", "Qwen Studio"} {
			if !strings.Contains(rail, name) {
				t.Fatalf("%dx%d navigation hides %q:\n%s", size.width, size.height, name, rail)
			}
		}
		view := ansi.Strip(model.View())
		if lipgloss.Width(view) > size.width || lipgloss.Height(view) > size.height {
			t.Fatalf("%dx%d picker overflows:\n%s", size.width, size.height, view)
		}
		if !strings.Contains(view, "EXPERIMENTAL") || !strings.Contains(view, "choose") {
			t.Fatalf("%dx%d lost selected status or action:\n%s", size.width, size.height, view)
		}
	}
}
