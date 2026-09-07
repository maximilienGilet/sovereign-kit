package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// Concatenating bordered strings puts the second button after the first
// button's bottom border rather than beside its label.
func TestRentalButtonsShareOneRow(t *testing.T) {
	for _, width := range []int{80, 110, 156} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			view := ansi.Strip(newRentalReview(rentalReviewFixture(), 120).costAndControls(false, width))
			for _, line := range strings.Split(view, "\n") {
				if strings.Contains(line, "Create paid instance") {
					if !strings.Contains(line, "Cancel") {
						t.Fatalf("buttons are not side by side:\n%s", view)
					}
					return
				}
			}
			t.Fatal("creation control missing")
		})
	}
}

// An embedded screen must not keep the standalone screen's controls/help in
// its scrollable body in addition to its fixed controls and the root footer.
func TestEmbeddedRentalHasOnlyOneControlArea(t *testing.T) {
	for _, size := range [][2]int{{30, 10}, {72, 24}, {150, 34}, {320, 80}, {320, 200}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config"), "user", "home", ApplicationDependencies{})
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			m.openPrompt(promptRequest{kind: promptCost, data: costPrompt{rentalReviewFixture(), 120}})
			view := ansi.Strip(m.View())
			lines := strings.Split(strings.TrimSuffix(view, "\n"), "\n")
			if len(lines) > size[1] {
				t.Fatal("review overflows terminal height")
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > size[0] {
					t.Fatal("review overflows terminal width")
				}
			}
			if strings.Contains(view, "enter confirm") || strings.Contains(view, "esc cancel") {
				t.Fatalf("standalone help leaked into embedded screen:\n%s", view)
			}
			if strings.Contains(view, "Create paid instance") && strings.Contains(view, "←→ CANCEL") {
				t.Fatalf("duplicate control areas:\n%s", view)
			}
			before := ansi.Strip(m.child.View())
			m.Update(tea.KeyMsg{Type: tea.KeyRight})
			after := ansi.Strip(m.child.View())
			if before == after {
				t.Fatal("selection has no visible indicator")
			}
			if strings.Count(view, "SOVEREIGN KIT") != 1 {
				t.Fatal("duplicate application header")
			}
			for _, label := range []string{"Create paid", "Cancel"} {
				if strings.Count(view, label) != 1 {
					t.Fatalf("expected one visible %s control:\n%s", label, view)
				}
			}
			if !strings.Contains(lines[len(lines)-1], "PgDn") {
				t.Fatal("root footer must expose review scrolling")
			}
			m.locked = true
			lockedLines := strings.Split(strings.TrimSuffix(ansi.Strip(m.View()), "\n"), "\n")
			lockedHelp := lockedLines[len(lockedLines)-1]
			if !strings.Contains(lockedHelp, "Ctrl+C") || !strings.Contains(lockedHelp, "PgDn") {
				t.Fatalf("locked review help clipped: %s", lockedHelp)
			}
			if os.Getenv("SOVKIT_CAPTURE_RENTAL") != "" && size == [2]int{150, 34} {
				for i, line := range lines {
					t.Logf("%02d %s", i+1, strings.TrimRight(line, " "))
				}
			}
		})
	}
}

func TestRentalFixedControlsStayVisibleWhenScrolled(t *testing.T) {
	child := tea.Model(newEmbeddedRentalReview(rentalReviewFixture(), 120))
	child, _ = child.Update(tea.WindowSizeMsg{Width: 30, Height: 6})
	for i := 0; i < 20; i++ {
		child, _ = child.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	}
	view := ansi.Strip(child.View())
	if !strings.Contains(view, "excludes storage") || !strings.Contains(view, "> [Cancel]") {
		t.Fatalf("review tail or fixed safe selection missing:\n%s", view)
	}
	_, cmd := child.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd().(rentalReviewResultMsg).Confirmed {
		t.Fatal("scrolling must not approve a paid instance")
	}
	child, _ = child.Update(tea.KeyMsg{Type: tea.KeyRight})
	_, cmd = child.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !cmd().(rentalReviewResultMsg).Confirmed {
		t.Fatal("explicit selection then Enter must still approve")
	}
}
