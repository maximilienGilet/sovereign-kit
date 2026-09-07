package cli

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestExitModalIsCenteredOverExistingScreen(t *testing.T) {
	m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config.toml"), "root", "home", ApplicationDependencies{})
	defer m.cleanup()
	m.width, m.height, m.screen = 120, 40, "working"
	m.exitConfirm, m.exitInstance = true, 4242
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	row := -1
	for i, line := range lines {
		if strings.Contains(line, "Quit · Vast #4242") {
			row = i
		}
	}
	if row < 12 || row > 23 {
		t.Fatalf("dialog is not vertically centered: title row %d", row)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "╭") {
		t.Fatal("missing modal border")
	}
}

func TestExitKeepsProvisioningBackgroundInPlace(t *testing.T) {
	m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config"), "root", "home", ApplicationDependencies{})
	defer m.cleanup()
	m.width, m.height, m.screen = 200, 70, "working"
	before := strings.Split(ansi.Strip(m.View()), "\n")
	m.exitConfirm, m.exitInstance = true, 4242
	after := strings.Split(ansi.Strip(m.View()), "\n")
	for y := 2; y < 68; y++ {
		// Outside the modal's horizontal footprint nothing is allowed to move.
		a, b := []rune(before[y]), []rune(after[y])
		for x := 0; x < 60; x++ {
			ar, br := ' ', ' '
			if x < len(a) {
				ar = a[x]
			}
			if x < len(b) {
				br = b[x]
			}
			if ar != br {
				t.Fatalf("background moved at %d,%d", x, y)
			}
		}
	}
}

func TestDestroyDialogVerticalChoicesAndRemoteData(t *testing.T) {
	m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config"), "root", "home", ApplicationDependencies{})
	defer m.cleanup()
	m.width, m.height, m.exitInstance, m.exitDestroy = 100, 40, 4242, true
	view := ansi.Strip(m.exitView())
	if !strings.Contains(view, "remote instance") {
		t.Fatal("ambiguous data deletion warning")
	}
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "Cancel") && strings.Contains(line, "Permanently destroy") {
			t.Fatal("choices are not vertical")
		}
	}
	m.exitKey(tea.KeyMsg{Type: tea.KeyDown})
	if !m.selected {
		t.Fatal("vertical navigation does not match vertical choices")
	}
}

func TestPaidReviewUsesCenteredConfirmation(t *testing.T) {
	m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config.toml"), "root", "home", ApplicationDependencies{})
	defer m.cleanup()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.openPrompt(promptRequest{kind: promptCost, data: costPrompt{rentalReviewFixture(), 120}})
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if strings.TrimSpace(lines[2]) != "" {
		t.Fatal("paid confirmation is pinned to top instead of centered")
	}
	if !strings.Contains(strings.Join(lines, "\n"), "Create paid") {
		t.Fatal("missing paid action")
	}
}
