package cli

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
)

// Enter has different consequences before and after the compact detail view.
func TestOfferBrowserFooterExplainsNextActionAtEachSize(t *testing.T) {
	for _, size := range []struct{ width, height int }{{80, 24}, {150, 40}} {
		m := newOfferBrowser(context.Background(), testRecipe(), func(context.Context, setup.OfferQuery) (setup.OfferSearchResult, error) {
			return browserFixture(), nil
		})
		t.Cleanup(m.Close)
		m.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
		m.Update(m.Init()())
		action := "Enter choose"
		if size.width < 108 {
			action = "Enter details"
		}
		footer := m.Footer()
		if !strings.Contains(footer, action) || !strings.Contains(footer, "i inspect") || !strings.Contains(footer, "Esc back") {
			t.Fatalf("%dx%d footer hides keyboard consequences: %s", size.width, size.height, footer)
		}
		if lipgloss.Width(footer) > m.width || lipgloss.Width(m.View()) > size.width || lipgloss.Height(m.View()) > size.height {
			t.Fatalf("%dx%d offer browser overflows", size.width, size.height)
		}
		if size.width < 108 {
			if browserKey(m, "enter") != nil || !m.details || !strings.Contains(m.Footer(), "Enter choose") {
				t.Fatal("compact Enter did not open details with an explicit choose action")
			}
		}
	}
}
