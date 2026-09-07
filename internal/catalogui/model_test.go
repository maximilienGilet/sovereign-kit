package catalogui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCatalogViewShowsLaunchableRecipeAndDetailRail(t *testing.T) {
	model := New([]Entry{{
		Name: "Qwen Studio", Kind: "text-generation", MinimumVRAMGB: 96, ContextWindow: 262144, Summary: "reference available", Status: "RECOMMENDED",
		ModelRepository: "RadixArk/Qwen3.8-27B-NVFP4", Evidence: "Historical reference", GPUModel: "Measure this exact offer before production",
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
		{Name: "Qwen Studio", ModelRepository: "qwen"},
		{Name: "Custom HF", ModelRepository: "custom"},
	})
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	view := updated.(Model).View()
	if !strings.Contains(view, "Custom HF") || !strings.Contains(view, "SOURCE") || !strings.Contains(view, "custom") {
		t.Fatalf("navigation did not select second entry:\n%s", view)
	}
}

func TestCatalogAdvertisesOnlyCatalogNavigation(t *testing.T) {
	view := New(DefaultEntries()).View()
	for _, unavailable := range []string{"Enter inspect", "Space hardware", "L launch"} {
		if strings.Contains(view, unavailable) {
			t.Fatalf("catalog advertises unavailable action %q:\n%s", unavailable, view)
		}
	}
	for _, available := range []string{"↑↓ browse", "/ filter", "q close"} {
		if !strings.Contains(view, available) {
			t.Fatalf("catalog missing navigation %q:\n%s", available, view)
		}
	}
}
