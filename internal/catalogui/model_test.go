package catalogui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCatalogViewShowsLaunchableRecipeAndDetailRail(t *testing.T) {
	model := New([]Entry{{
		Name: "Qwen Studio", Kind: "text-generation", VRAM: "96 GB", Context: "262K", Speed: "reference available", Cost: "$1.20/h", Status: "RECOMMENDED",
		Detail: Detail{Source: "RadixArk/Qwen3.8-27B-NVFP4", Confidence: "Historical reference", Notes: "Measure this exact offer before production"},
	}})
	view := model.View()
	for _, expected := range []string{"CATALOG", "RECIPE", "Qwen Studio", "HARDWARE", "RECOMMENDED", "Historical reference"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("catalog view missing %q:\n%s", expected, view)
		}
	}
}

func TestCatalogNavigationChangesTheDetailRail(t *testing.T) {
	model := New([]Entry{
		{Name: "Qwen Studio", Detail: Detail{Source: "qwen"}},
		{Name: "Custom HF", Detail: Detail{Source: "custom"}},
	})
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	view := updated.(Model).View()
	if !strings.Contains(view, "Custom HF") || !strings.Contains(view, "SOURCE") || !strings.Contains(view, "custom") {
		t.Fatalf("navigation did not select second entry:\n%s", view)
	}
}
