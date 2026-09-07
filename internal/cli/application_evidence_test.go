package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
)

func TestRootRecipeEvidencePagingReachesTailAndReturnsToTop(t *testing.T) {
	for _, size := range [][2]int{{30, 10}, {72, 24}, {150, 34}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config.toml"), "alice", "home", ApplicationDependencies{})
			defer m.cleanup()
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			r := testRecipe()
			r.Profile.Evidence = strings.Repeat("Long verified fixture evidence. ", 100)
			r.Profile.UseWhen = []string{"EVIDENCE-END"}
			m.openPrompt(promptRequest{kind: promptWorkload, data: []recipe.Recipe{r}, reply: make(chan any, 1)})
			before := m.View()
			if size[0] == 30 {
				m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
				if m.View() == before {
					t.Fatal("compact recipe body lost outer paging")
				}
			}
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
			top := ansi.Strip(m.View())
			if strings.Contains(top, "EVIDENCE-END") {
				t.Fatal("fixture evidence was not long enough")
			}
			for i := 0; i < 200 && !strings.Contains(ansi.Strip(m.View()), "EVIDENCE-END"); i++ {
				m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
			}
			if view := ansi.Strip(m.View()); !strings.Contains(view, "EVIDENCE-END") {
				t.Fatalf("evidence tail unreachable:\n%s", view)
			}
			for i := 0; i < 200; i++ {
				m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
			}
			if view := ansi.Strip(m.View()); view != top {
				t.Fatalf("PgUp did not restore top:\n%s\nwant:\n%s", view, top)
			}
			m.Update(tea.KeyMsg{Type: tea.KeyEsc})
			if size[0] == 30 {
				before = m.View()
				m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
				if m.View() == before {
					t.Fatal("closing evidence lost outer body paging")
				}
			}
			before = m.View()
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
			if view := m.View(); view != before {
				t.Fatalf("i close did not restore original recipe body sizing:\n%s", view)
			}
		})
	}
}

func TestLockedCostFooterOffersStopInsteadOfUnavailableBack(t *testing.T) {
	for _, size := range [][2]int{{30, 10}, {150, 34}} {
		m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config.toml"), "alice", "home", ApplicationDependencies{})
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.locked = true
		m.openPrompt(promptRequest{kind: promptCost, data: costPrompt{view: rentalReviewFixture(), disk: 120}, reply: make(chan any, 1)})
		view := ansi.Strip(m.View())
		if strings.Contains(view, "Esc back") || !strings.Contains(view, "Ctrl+C") {
			t.Fatalf("misleading locked controls:\n%s", view)
		}
		m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		if m.pending == nil || m.pending.kind != promptCost {
			t.Fatal("locked review navigated back")
		}
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		if !m.exitConfirm {
			t.Fatal("visible stop did not request exit")
		}
		m.cleanup()
	}
}
