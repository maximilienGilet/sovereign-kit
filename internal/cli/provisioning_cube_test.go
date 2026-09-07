package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/muesli/termenv"
)

func TestCubeLightMovesWithoutGeometryJitter(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	a := provisioningCube(time.Second, time.Second, true, "")
	b := provisioningCube(2*time.Second, time.Second, true, "")
	if a == b {
		t.Fatal("edge lighting did not animate")
	}
	if ansi.Strip(a) != ansi.Strip(b) {
		t.Fatal("cube geometry moved")
	}
	for _, marker := range []string{"!", "✓"} {
		a = provisioningCube(time.Second, time.Second, true, marker)
		b = provisioningCube(2*time.Second, time.Second, true, marker)
		if a != b {
			t.Fatal("terminal-state cube is not frozen")
		}
		if !strings.Contains(ansi.Strip(a), marker) {
			t.Fatal("state marker absent")
		}
	}
}

func TestProvisioningCubeComposition(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	m.screen = "working"
	m.applyProgress(setup.Progress{Stage: setup.ProgressWaiting, InstanceID: 12345})
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	view := ansi.Strip(m.View())
	lines := strings.Split(view, "\n")
	activeRow, activeCol, futureCol, warningCol := -1, -1, -1, -1
	for y, line := range lines {
		if i := strings.Index(line, "> Waiting"); i >= 0 {
			activeRow, activeCol = y, ansi.StringWidth(line[:i])
		}
		if i := strings.Index(line, "○ Secure SSH host"); i >= 0 {
			futureCol = ansi.StringWidth(line[:i])
		}
		if i := strings.Index(line, "Instance #12345"); i >= 0 {
			warningCol = i
		}
	}
	if activeRow < 8 || activeCol < 60 || activeCol != futureCol || warningCol < 10 {
		t.Fatalf("composition not centered / aligned: active=%d,%d future=%d warning=%d\n%s", activeRow, activeCol, futureCol, warningCol, view)
	}
	if !strings.Contains(view, "PROVISIONING") {
		t.Fatal("missing section title")
	}
}

func TestProvisioningCubeBounds(t *testing.T) {
	l := provisioningLoader{label: "Waiting for the instance…", started: time.Now(), completed: []string{"Instance created"}, future: []string{"Secure SSH host", "Launch inference server", "Save configuration", "Verify connection"}}
	l.now = l.started.Add(time.Second)
	for _, size := range [][2]int{{30, 2}, {56, 22}, {80, 24}, {90, 20}, {150, 34}} {
		view := l.view(size[0], size[1], false, "")
		if strings.Count(view, "\n")+1 > size[1] {
			t.Fatalf("height overflow %v", size)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("width overflow %v", size)
			}
		}
	}
}
