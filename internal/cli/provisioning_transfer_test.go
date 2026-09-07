package cli

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/muesli/termenv"
)

func TestTransferBarUsesMeasuredBytesAndClearsAfterVerification(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	m.screen = "working"
	m.applyProgress(setup.Progress{Stage: setup.ProgressLaunching, InstanceID: 42})
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 42})
	at := time.Now()
	m.applyServerLogs(setup.ServerLogs{Text: `SOVKIT_DOWNLOAD {"current":400,"total":1000}`, CheckedAt: at})
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "40.0%") || !strings.Contains(view, "MODEL DOWNLOAD") {
		t.Fatalf("missing measured bar: %s", view)
	}
	m.applyServerLogs(setup.ServerLogs{Text: `SOVKIT_DOWNLOAD {"current":400,"total":0}`, CheckedAt: at.Add(time.Second)})
	view = ansi.Strip(m.View())
	if strings.Contains(view, "40.0%") || strings.Contains(view, "100.0%") || !strings.Contains(view, "total unknown") {
		t.Fatalf("invented percentage: %s", view)
	}
	m.applyServerLogs(setup.ServerLogs{Text: "SOVKIT_DOWNLOAD {\"current\":1000,\"total\":1000}\nVerifying GGUF SHA256", CheckedAt: at.Add(2 * time.Second)})
	if strings.Contains(ansi.Strip(m.View()), "100.0%") {
		t.Fatal("verification retained download completion bar")
	}
}

func TestMeasuredTransferKeepsHighWaterAndCompletedForge(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	m.applyProgress(setup.Progress{Stage: setup.ProgressLaunching, InstanceID: 42})
	at := time.Now()
	m.applyServerLogs(setup.ServerLogs{Text: `SOVKIT_DOWNLOAD {"current":600,"total":1000}`, CheckedAt: at})
	m.applyServerLogs(setup.ServerLogs{Text: `SOVKIT_DOWNLOAD {"current":400,"total":1000}`, CheckedAt: at.Add(time.Second)})
	if m.loader.transferRatioHigh != .6 {
		t.Fatalf("measured construction regressed: %.2f", m.loader.transferRatioHigh)
	}
	m.applyServerLogs(setup.ServerLogs{Text: `SOVKIT_DOWNLOAD {"current":1000,"total":1000}`, CheckedAt: at.Add(2 * time.Second)})
	m.applyServerLogs(setup.ServerLogs{Text: "Verifying GGUF SHA256", CheckedAt: at.Add(3 * time.Second)})
	if !m.loader.transferCompleted || m.loader.transferActive {
		t.Fatalf("completion not retained through verification: %+v", m.loader)
	}
}

func TestCompletedDownloadIsRetainedWhenVerificationArrivesInSameSnapshot(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	m.applyProgress(setup.Progress{Stage: setup.ProgressLaunching, InstanceID: 42})
	m.applyServerLogs(setup.ServerLogs{Text: "SOVKIT_DOWNLOAD {\"current\":1000,\"total\":1000}\nVerifying GGUF SHA256", CheckedAt: time.Now()})
	if !m.loader.transferCompleted || m.loader.transferRatioHigh != 1 {
		t.Fatalf("completion disappeared at verification: %+v", m.loader)
	}
}

func TestInvalidTransferDoesNotReplaceMeasuredForgeWithIntake(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	m.applyProgress(setup.Progress{Stage: setup.ProgressLaunching, InstanceID: 42})
	at := time.Now()
	m.applyServerLogs(setup.ServerLogs{Text: `SOVKIT_DOWNLOAD {"current":600,"total":1000}`, CheckedAt: at})
	m.applyServerLogs(setup.ServerLogs{Text: `SOVKIT_DOWNLOAD {"current":1100,"total":1000}`, CheckedAt: at.Add(time.Second)})
	mode, ratio := m.loader.forgeState()
	if mode != forgeMeasured || ratio != .6 {
		t.Fatalf("invalid counter erased measured construction: mode=%d ratio=%.2f", mode, ratio)
	}
}

func TestUnknownTransferHasBytesButNoIndeterminateMeter(t *testing.T) {
	l := provisioningLoader{started: time.Unix(0, 0), now: time.Unix(1, 0), transferCurrent: 1024, transferActive: true}
	view := ansi.Strip(l.transferView(30, false, true))
	if !strings.Contains(view, "1.0 KiB · total unknown") || strings.ContainsAny(view, "━█") {
		t.Fatalf("unknown transfer rendered a fake meter: %q", view)
	}
}

func TestLargeForgeDoesNotDuplicateMeasuredTransferBar(t *testing.T) {
	l := provisioningLoader{label: "Downloading model", started: time.Unix(0, 0), now: time.Unix(1, 0), transferActive: true, transferCurrent: 600, transferTotal: 1000, transferRatioHigh: .6}
	view := ansi.Strip(l.view(108, 30, false, ""))
	if !strings.Contains(view, "600 B / 1000 B · 60.0%") {
		t.Fatalf("measured detail missing: %s", view)
	}
	if strings.Contains(view, "████") {
		t.Fatalf("duplicate horizontal transfer bar remains beside forge: %s", view)
	}
}

func TestLongDownloadLogRetainsNewestCounterAndVerification(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	m.applyProgress(setup.Progress{Stage: setup.ProgressLaunching, InstanceID: 42})
	var log strings.Builder
	for i := 0; i < 240; i++ {
		fmt.Fprintf(&log, "SOVKIT_DOWNLOAD {\"current\":%d,\"total\":240}\n", i)
	}
	at := time.Now()
	m.applyServerLogs(setup.ServerLogs{Text: log.String(), CheckedAt: at})
	if !m.loader.transferActive || m.loader.transferCurrent != 239 {
		t.Fatalf("newest counter lost: %d", m.loader.transferCurrent)
	}
	m.applyServerLogs(setup.ServerLogs{Text: log.String() + "Verifying GGUF SHA256", CheckedAt: at.Add(time.Second)})
	if m.loader.transferActive {
		t.Fatal("verification lost after log truncation")
	}
}

func TestProviderDetailDoesNotTurnPollingIntoProgress(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	m.applyProgress(setup.Progress{Stage: setup.ProgressWaiting, InstanceID: 42})
	at := time.Now()
	a := setup.Activity{InstanceID: 42, Status: "loading", Message: "layer: Download complete", Detail: "layer: Retrying in 5 seconds", CheckedAt: at}
	m.applyActivity(a)
	a.CheckedAt = at.Add(time.Minute)
	m.applyActivity(a)
	m.loader.now = at.Add(time.Minute)
	view := m.loader.activityView(60, 6, false)
	if strings.Count(view, "Retrying") != 1 || !strings.Contains(view, "last change 60s ago") {
		t.Fatalf("stale daemon details manufactured progress: %s", view)
	}
	a.Detail = strings.Repeat("old daemon line\n", 200) + "latest layer: Extracting"
	a.CheckedAt = at.Add(2 * time.Minute)
	m.applyActivity(a)
	if !strings.Contains(m.loader.activityView(60, 6, false), "latest layer: Extracting") {
		t.Fatal("UI truncated newest daemon detail")
	}
}

func TestStepBarShimmerFreezesAndDoesNotChangeWidth(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	l := provisioningLoader{started: time.Unix(0, 0), now: time.Unix(1, 0), completed: []string{"Created"}, future: []string{"SSH", "Model"}}
	a, frozen := l.stepBar(48, true, false), l.stepBar(48, true, true)
	l.now = l.now.Add(time.Second)
	b := l.stepBar(48, true, false)
	if a == b || ansi.Strip(a) != ansi.Strip(b) || l.stepBar(48, true, true) != frozen {
		t.Fatal("shimmer failed to animate/freeze without shifting layout")
	}
}

func TestMeasuredTransferDoesNotOverflowResponsiveScreens(t *testing.T) {
	for _, size := range [][2]int{{30, 10}, {56, 24}, {108, 30}, {150, 42}} {
		m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
		m.screen = "working"
		m.applyProgress(setup.Progress{Stage: setup.ProgressLaunching, InstanceID: 42})
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.applyServerLogs(setup.ServerLogs{Text: `SOVKIT_DOWNLOAD {"current":4000000000,"total":10000000000}`, CheckedAt: time.Now()})
		view := ansi.Strip(m.View())
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("%v overflow: %s", size, line)
			}
		}
		if !strings.Contains(view, "#42") || !strings.Contains(view, "Ctrl+C") {
			t.Fatalf("transfer displaced safety controls at %v", size)
		}
		if size == [2]int{30, 10} && !strings.Contains(view, "3.73 GiB") {
			t.Fatalf("compact transfer hid measured bytes: %s", view)
		}
	}
}

func TestProviderPollingDoesNotResetLastChangeAge(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	m.screen = "working"
	m.applyProgress(setup.Progress{Stage: setup.ProgressWaiting, InstanceID: 42})
	at := time.Now()
	m.applyActivity(setup.Activity{InstanceID: 42, Status: "loading", Message: "layer: Download complete", CheckedAt: at})
	m.applyActivity(setup.Activity{InstanceID: 42, Status: "loading", Message: "layer: Download complete", CheckedAt: at.Add(240 * time.Second)})
	m.loader.now = at.Add(243 * time.Second)
	view := ansi.Strip(m.loader.activityView(60, 8, false))
	if !strings.Contains(view, "checked 3s ago") || !strings.Contains(view, "last change 243s ago") {
		t.Fatalf("polling hides stagnation: %s", view)
	}
}

func TestMilestoneBarIsNotATimePercentage(t *testing.T) {
	l := provisioningLoader{completed: []string{"Instance created"}, future: []string{"Verify SSH", "Launch", "Save", "Connect"}, label: "Waiting", started: time.Now()}
	l.now = l.started.Add(time.Second)
	view := ansi.Strip(l.view(108, 32, false, ""))
	if !strings.Contains(view, "1/6 steps completed") || strings.Contains(view, "%") {
		t.Fatalf("missing honest step bar: %s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) > 108 {
			t.Fatal("bar overflow")
		}
	}
}
