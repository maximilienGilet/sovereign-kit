package dashboardui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// Losing the content bound makes cards span an unreadably wide terminal.
func TestLargeDashboardCentersBoundedContentWithoutChangingTerminalSize(t *testing.T) {
	m := focusInspectorFixture(300)
	m.height = 90
	view := ansi.Strip(m.View())
	for _, line := range strings.Split(view, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.HasPrefix(line, strings.Repeat(" ", 80)) {
			t.Fatalf("content is not centered: %q", line)
		}
		if ansi.StringWidth(strings.TrimSpace(line)) > 140 {
			t.Fatalf("content exceeds readable width: %q", line)
		}
	}
	if m.width != 300 {
		t.Fatal("render changed terminal dimensions")
	}
}

// Tall terminals should give the complete dashboard balanced breathing room.
func TestTallDashboardCentersCompleteViewVertically(t *testing.T) {
	m := focusInspectorFixture(300)
	m.height = 90
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	first, last := -1, -1
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 10 || len(lines) != 90 || len(lines)-1-last < first || len(lines)-1-last > first+1 {
		t.Fatalf("unbalanced vertical margins: top=%d bottom=%d height=%d", first, len(lines)-1-last, len(lines))
	}
	if !strings.Contains(strings.Join(lines, "\n"), "OpenCode") {
		t.Fatal("centering lost endpoint actions")
	}
	if m.height != 90 || m.width != 300 {
		t.Fatal("centering mutated dimensions")
	}
}

func TestTallOverflowingDashboardKeepsScrollViewportUnpadded(t *testing.T) {
	m := focusInspectorFixture(120).WithServerLogs(strings.Repeat("long snapshot ", 500), time.Now())
	m.height, m.logsExpanded = 40, true
	for _, scroll := range []int{0, 20} {
		m.scroll, m.manualScroll = scroll, true
		view := ansi.Strip(m.View())
		if !strings.HasPrefix(view, "PRIVATE ENDPOINT\n") || len(strings.Split(view, "\n")) != 40 {
			t.Fatalf("scroll=%d: overflowing viewport was padded or resized", scroll)
		}
	}
}

// Missing counters cannot justify idle; only a measured zero active count can.
func TestDashboardDistinguishesWaitingFromMeasuredIdle(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 40}} {
		m := NewEndpoint(context.Background(), endpointFixture(), 42, Dependencies{}).SetHealthy(true)
		m.width, m.height = size[0], size[1]
		if view := ansi.Strip(m.View()); !strings.Contains(view, "Waiting for telemetry") || strings.Contains(view, "Idle") {
			t.Fatalf("unknown activity mislabeled:\n%s", view)
		}
		m.stats.Active = throughputValue(0)
		m.stats.Queued = throughputValue(0)
		if view := ansi.Strip(m.View()); !strings.Contains(view, "Idle") || strings.Contains(view, "0 tok/s") {
			t.Fatalf("measured idle not distinguished from missing throughput:\n%s", view)
		}
	}
}

// A missing toggle either exposes noisy snapshots by default or makes them inaccessible.
func TestDashboardLogsStartFoldedAndToggleWithoutLosingEndpointActions(t *testing.T) {
	m := focusInspectorFixture(120).WithServerLogs("unique log snapshot", time.Now())
	m.height = 40
	if strings.Contains(ansi.Strip(m.View()), "unique log snapshot") {
		t.Fatal("logs expanded by default")
	}
	if !strings.Contains(m.ActionHint(), "l logs") {
		t.Fatal("logs toggle not discoverable")
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = next.(Model)
	if cmd != nil {
		t.Fatal("log toggle launched external work")
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "unique log snapshot") || !strings.Contains(view, "OpenCode") {
		t.Fatalf("expanded logs lost content or actions:\n%s", view)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if strings.Contains(ansi.Strip(next.(Model).View()), "unique log snapshot") {
		t.Fatal("second toggle did not fold logs")
	}
}

// Fixed-height plots waste large terminals; stacked lower cards push actions off screen.
func TestWideDashboardUsesHeightForPlotAndPairsLowerCards(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {180, 55}, {300, 90}} {
		m := focusInspectorFixture(size[0])
		m.height = size[1]
		view := ansi.Strip(m.View())
		plotStart, plotEnd := -1, -1
		paired := false
		for i, line := range strings.Split(view, "\n") {
			if strings.Contains(line, "Current generation") {
				plotStart = i + 1
			}
			if strings.Contains(line, "-10m") {
				plotEnd = i
			}
			if strings.Contains(line, "Session & instance") && strings.Contains(line, "Endpoint & integrations") {
				paired = true
			}
		}
		if plotEnd-plotStart+1 < 10 || plotEnd-plotStart+1 > 14 {
			t.Fatalf("%v: chart height %d, want 10..14:\n%s", size, plotEnd-plotStart+1, view)
		}
		if !paired {
			t.Fatalf("%v: lower cards not paired:\n%s", size, view)
		}
		if !strings.Contains(view, "OpenCode") {
			t.Fatalf("%v: endpoint actions not visible", size)
		}
	}
}

func TestPolishedDashboardGeometryKeepsActionsAcrossTerminalSizes(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 40}, {180, 55}, {300, 90}} {
		m := focusInspectorFixture(size[0])
		m.height = size[1]
		for _, expanded := range []bool{false, true} {
			m.logsExpanded = expanded
			m = m.WithServerLogs(strings.Repeat("long server line ", 30), time.Now())
			view := ansi.Strip(m.View())
			if len(strings.Split(view, "\n")) > size[1] {
				t.Fatalf("%v height overflow", size)
			}
			for _, line := range strings.Split(view, "\n") {
				if ansi.StringWidth(line) > size[0] {
					t.Fatalf("%v width overflow: %q", size, line)
				}
			}
			for i := 0; i < 5; i++ {
				m, _ = key(m, tea.KeyDown)
			}
			if !strings.Contains(ansi.Strip(m.View()), "› OpenCode") {
				t.Fatalf("%v selected action hidden", size)
			}
		}
	}
}
