package cli

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"strings"
	"testing"
	"time"
)

func TestProviderActivityHistoryAndHeartbeat(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	m.screen = "working"
	m.applyProgress(setup.Progress{Stage: setup.ProgressWaiting, InstanceID: 987})
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	at := time.Now()
	for i := 0; i < 6; i++ {
		m.applyActivity(setup.Activity{InstanceID: 987, Status: "loading", Message: fmt.Sprintf("layer%d: Verifying Checksum", i), CheckedAt: at.Add(time.Duration(i) * time.Second)})
	}
	m.applyActivity(setup.Activity{InstanceID: 987, Status: "loading", Message: "layer5: Verifying Checksum", CheckedAt: at.Add(7 * time.Second)})
	m.loader.now = at.Add(9 * time.Second)
	view := ansi.Strip(m.View())
	if strings.Contains(view, "layer1:") || !strings.Contains(view, "layer2:") || strings.Count(view, "layer5:") != 1 || !strings.Contains(view, "checked 2s ago") {
		t.Fatalf("bad history/heartbeat:\n%s", view)
	}
	if m.progressStage != setup.ProgressWaiting {
		t.Fatal("activity advanced setup")
	}
	m.applyActivity(setup.Activity{InstanceID: 555, Status: "running", Message: "wrong instance", CheckedAt: at})
	if strings.Contains(ansi.Strip(m.View()), "wrong instance") {
		t.Fatal("foreign activity accepted")
	}
	for _, size := range [][2]int{{30, 10}, {56, 24}, {100, 24}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view = ansi.Strip(m.View())
		for _, want := range []string{"layer5", "987", "billing", "Ctrl+C"} {
			if !strings.Contains(view, want) {
				t.Fatalf("%v missing %s:\n%s", size, want, view)
			}
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("activity overflow")
			}
		}
	}
}

func TestSSHRetryFeedbackIsVisibleOnlyDuringHostKeyStage(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	m.screen = "working"
	m.applyProgress(setup.Progress{Stage: setup.ProgressHostKeys, InstanceID: 987})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	at := time.Now()
	m.applyActivity(setup.Activity{Stage: setup.ProgressHostKeys, InstanceID: 987, Status: "SSH", Message: "SSH not ready · attempt 2 failed · retrying in 5s", CheckedAt: at})
	if !strings.Contains(ansi.Strip(m.View()), "attempt 2 failed") {
		t.Fatal("SSH retry feedback hidden")
	}
	m.applyActivity(setup.Activity{InstanceID: 987, Status: "loading", Message: "stale provider event", CheckedAt: at.Add(time.Second)})
	if strings.Contains(ansi.Strip(m.View()), "stale provider event") {
		t.Fatal("provider event overwrote SSH feedback")
	}
}
