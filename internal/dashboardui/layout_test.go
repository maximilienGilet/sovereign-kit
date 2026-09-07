package dashboardui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/clientprofile"
	"github.com/maximilienGilet/sovereign-kit/internal/endpointstats"
)

func TestActivityPollingIsChainedAndMissingValuesStayUnavailable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reads := 0
	active, decode := 0.0, 25.5
	m := NewEndpoint(ctx, endpointFixture(), 42, Dependencies{Stats: func(context.Context, string) endpointstats.Snapshot {
		reads++
		return endpointstats.Snapshot{CheckedAt: time.Now(), Active: &active, DecodeTokensPerSecond: &decode}
	}, Discover: func(context.Context, string, clientprofile.Metadata) clientprofile.Endpoint { return endpointFixture() }}).SetHealthy(true)
	m.width, m.height = 120, 40
	next, cmd := m.Update(m.Init()())
	m = next.(Model)
	if cmd == nil || reads != 0 {
		t.Fatal("discovery must schedule one asynchronous metrics read")
	}
	next, duplicate := m.Update(WorkResult{Kind: "discover", Endpoint: endpointFixture()})
	m = next.(Model)
	if duplicate != nil {
		t.Fatal("rediscovery created an overlapping metrics worker")
	}
	next, wait := m.Update(cmd())
	m = next.(Model)
	if reads != 1 || wait == nil {
		t.Fatal("metrics completion did not schedule next interval")
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"Active requests", "25.5", "not reported"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing measurement %q", want)
		}
	}
	if strings.Contains(view, "Queued requests  0") {
		t.Fatal("absent queue count shown as measured zero")
	}
	cancel()
	if msg := wait(); msg != nil {
		t.Fatalf("canceled timer produced a poll: %T", msg)
	}
}

func TestStandardTerminalShowsEndpointActivityAndInstanceTogether(t *testing.T) {
	m := NewEndpoint(context.Background(), endpointFixture(), 42, Dependencies{}).SetHealthy(true)
	m.width, m.height = 80, 24
	view := ansi.Strip(m.mainView())
	for _, want := range []string{"owner/solo", "32768", "unavailable", "Session & instance", "#42", "Session events"} {
		if !strings.Contains(view, want) {
			t.Errorf("standard viewport missing %q:\n%s", want, view)
		}
	}
}

func TestEmbeddedStandardDashboardKeepsCoreFactsVisible(t *testing.T) {
	m := NewEndpoint(context.Background(), endpointFixture(), 42, Dependencies{}).SetHealthy(true)
	m.width, m.height = 80, 20
	m = m.WithSession(SessionInfo{Provider: "vast", StartedAt: time.Date(2026, 9, 4, 10, 15, 0, 0, time.UTC)})
	view := ansi.Strip(m.mainView())
	for _, want := range []string{"owner/solo", "OpenCode", "32768", "unavailable", "#42"} {
		if !strings.Contains(view, want) {
			t.Errorf("embedded dashboard missing %q:\n%s", want, view)
		}
	}
}

func TestCardFillsAllocatedColumnsWithoutDoubleWrapping(t *testing.T) {
	view := card("Example", strings.Repeat("a", 26), 30)
	lines := strings.Split(view, "\n")
	if len(lines) != 4 {
		t.Fatalf("content wrapped twice: %q", view)
	}
	for _, line := range lines {
		if ansi.StringWidth(line) != 30 {
			t.Fatalf("card width %d, want 30", ansi.StringWidth(line))
		}
	}
}

func TestWideFocusInspectorHasOneAuthoritativeLocationPerFact(t *testing.T) {
	m := focusInspectorFixture(120)
	view := ansi.Strip(m.mainView())

	statusIndex := strings.Index(view, "Status")
	performanceIndex := strings.Index(view, "Generation throughput")
	if statusIndex < 0 || performanceIndex < 0 || statusIndex >= performanceIndex {
		t.Fatalf("status strip does not precede performance area:\n%s", view)
	}
	chartBesideInspector := false
	for _, line := range strings.Split(view, "\n") {
		chartIndex, inspectorIndex := strings.Index(line, "Generation throughput"), strings.Index(line, "Inspector")
		if chartIndex >= 0 && inspectorIndex > chartIndex {
			chartBesideInspector = true
			break
		}
	}
	if !chartBesideInspector {
		t.Fatalf("wide chart is not left of inspector:\n%s", view)
	}

	for label, want := range map[string]string{
		"current generation": "Current generation  30 tok/s",
		"generation average": "Generation average  25 tok/s",
		"generation peak":    "Generation peak     40 tok/s",
		"prompt throughput":  "Prompt throughput   123.4 tok/s",
		"KV usage":           "KV cache usage      42.5%",
		"context":            "Context window      32768 tokens",
		"output":             "Output limit        4096 tokens",
		"active requests":    "Active requests  2",
		"queued requests":    "Queued requests  1",
		"instance":           "#42",
		"endpoint":           "http://127.0.0.1:30000/v1",
		"model":              "owner/solo",
	} {
		if count := strings.Count(view, want); count != 1 {
			t.Errorf("%s fact %q occurs %d times, want exactly once:\n%s", label, want, count, view)
		}
	}
	if !containsBraille(view) {
		t.Fatalf("wide throughput panel has no Braille plot:\n%s", view)
	}
}

func TestThroughputLayoutStacksAtMediumWidthWithoutOverflow(t *testing.T) {
	m := focusInspectorFixture(80)
	view := ansi.Strip(m.mainView())
	chartLine, inspectorLine := -1, -1
	for index, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) > m.width {
			t.Fatalf("medium layout overflows width %d: %q", m.width, line)
		}
		if strings.Contains(line, "Generation throughput") {
			chartLine = index
		}
		if strings.Contains(line, "Inspector") {
			inspectorLine = index
		}
	}
	if chartLine < 0 || inspectorLine <= chartLine {
		t.Fatalf("medium chart and inspector are not stacked in order:\n%s", view)
	}
	if !containsBraille(view) {
		t.Fatalf("medium throughput panel unexpectedly omitted plot:\n%s", view)
	}
}

func TestThroughputLayoutUsesTextFallbackAtSmallestWidth(t *testing.T) {
	m := focusInspectorFixture(12)
	view := ansi.Strip(m.mainView())
	for _, want := range []string{"Current", "30 tok/s", "Average", "25 tok/s", "Peak", "40 tok/s"} {
		if !strings.Contains(view, want) {
			t.Errorf("narrow textual fallback missing %q:\n%s", want, view)
		}
	}
	if containsBraille(view) {
		t.Fatalf("narrow layout retained an unusable Braille plot:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) > m.width {
			t.Fatalf("narrow layout overflows width %d: %q", m.width, line)
		}
	}
}

func TestThroughputLayoutExpiresOldHistoryWithoutServerTimestamp(t *testing.T) {
	m := NewEndpoint(context.Background(), endpointFixture(), 42, Dependencies{}).SetHealthy(true)
	expired := 72.0
	m.throughput.add(time.Now().Add(-throughputWindow-time.Minute), &expired)

	view := ansi.Strip(m.inspectorBody(80))
	for _, want := range []string{"Generation average  not reported", "Generation peak     not reported"} {
		if !strings.Contains(view, want) {
			t.Errorf("expired live-window value remained visible; missing %q:\n%s", want, view)
		}
	}
}

func TestThroughputLayoutNarrowFallbackShowsTelemetryInterruption(t *testing.T) {
	m := NewEndpoint(context.Background(), endpointFixture(), 42, Dependencies{}).SetHealthy(true)
	checkedAt := time.Now()
	reported := 15.0
	m.width = 34
	m.throughput.add(checkedAt.Add(-time.Second), &reported)
	m.stats = endpointstats.Snapshot{CheckedAt: checkedAt, Problem: "metrics unavailable"}

	throughput := ansi.Strip(m.throughputPanelBody(m.width))
	if !strings.Contains(throughput, "Telemetry interrupted") || !strings.Contains(throughput, "not reported") {
		t.Fatalf("narrow fallback did not distinguish interrupted current telemetry:\n%s", throughput)
	}
	if inspector := ansi.Strip(m.inspectorBody(m.width)); !strings.Contains(inspector, "Average\n15 tok/s") {
		t.Fatalf("narrow interruption discarded the retained reported history:\n%s", inspector)
	}
}

func TestThroughputWindowAdvancesWhileLastReportIsStale(t *testing.T) {
	m := NewEndpoint(context.Background(), endpointFixture(), 42, Dependencies{}).SetHealthy(true)
	expired := 72.0
	checkedAt := time.Now().Add(-throughputWindow - time.Minute)
	m.stats = endpointstats.Snapshot{CheckedAt: checkedAt, DecodeTokensPerSecond: &expired}
	m.throughput.add(checkedAt, &expired)
	before := time.Now()
	now := m.throughputReferenceTime()
	if now.Before(before) || now.After(time.Now()) {
		t.Fatalf("live domain follows stale report time: %s", now)
	}
	if view := ansi.Strip(m.inspectorBody(80)); !strings.Contains(view, "Generation average  not reported") || !strings.Contains(view, "Generation peak     not reported") {
		t.Fatalf("delayed polling froze expired samples in live summary:\n%s", view)
	}
	if view := ansi.Strip(m.throughputPanelBody(80)); !strings.Contains(view, "Throughput unavailable") || containsBraille(view) {
		t.Fatalf("delayed polling froze expired samples in live chart:\n%s", view)
	}
}

func TestCompactEmbeddedViewportShowsCoreFactsAndLiveActivityOnce(t *testing.T) {
	for _, height := range []int{20, 24} {
		m := focusInspectorFixture(80)
		m.height = height
		view := ansi.Strip(m.View())
		for _, fact := range []string{"owner/solo", "http://127.0.0.1:30000/v1", "#42", "Model verified", "Active requests  2", "Queued requests  1", "Current generation  30 tok/s", "Average 25 tok/s", "Peak 40 tok/s", "Prompt 123.4 tok/s", "KV 42.5%", "Context 32768 tokens", "Output 4096 tokens", "OpenCode", "-10m", "-5m", "now"} {
			if count := strings.Count(view, fact); count != 1 {
				t.Errorf("height %d: fact %q occurs %d times:\n%s", height, fact, count, view)
			}
		}
		if !containsBraille(view) {
			t.Errorf("height %d: initial viewport lost live chart:\n%s", height, view)
		}
		previous := -1
		for _, section := range []string{"Status", "Generation throughput", "Inspector", "Session & instance", "Endpoint & integrations"} {
			index := strings.Index(view, section)
			if index <= previous {
				t.Errorf("height %d: section %q missing or out of order:\n%s", height, section, view)
			}
			previous = index
		}
	}
}

func TestFocusInspectorInitialViewKeepsFirstGlanceVisible(t *testing.T) {
	m := NewEndpoint(context.Background(), endpointFixture(), 42, Dependencies{}).SetHealthy(true)
	m.width, m.height = 80, 24
	view := ansi.Strip(m.View())
	for _, want := range []string{"Status", "Generation throughput"} {
		if !strings.Contains(view, want) {
			t.Errorf("initial first-glance dashboard missing %q:\n%s", want, view)
		}
	}
	for _, want := range []string{"› Base URL:", "owner/solo", "#42", "Current generation", "-10m", "now"} {
		if !strings.Contains(view, want) {
			t.Errorf("initial compact dashboard missing %q:\n%s", want, view)
		}
	}
}

func TestFocusInspectorAutoScrollKeepsBaseURLVisibleAfterMovingUp(t *testing.T) {
	m := NewEndpoint(context.Background(), endpointFixture(), 42, Dependencies{}).SetHealthy(true)
	m.width, m.height, m.cursor = 80, 24, 1
	m, _ = key(m, tea.KeyUp)
	if view := ansi.Strip(m.View()); !strings.Contains(view, "› Base URL:") {
		t.Fatalf("moving from Model to Base URL hid the selected action:\n%s", view)
	}
}

func focusInspectorFixture(width int) Model {
	now := time.Now()
	active, queued, prompt, kv := 2.0, 1.0, 123.4, 0.425
	m := NewEndpoint(context.Background(), endpointFixture(), 42, Dependencies{}).SetHealthy(true)
	m.width, m.height = width, 100
	m.stats = endpointstats.Snapshot{
		CheckedAt: now, Active: &active, Queued: &queued,
		DecodeTokensPerSecond: throughputValue(30), PromptTokensPerSecond: &prompt, KVUsageRatio: &kv,
	}
	for index, value := range []float64{10, 20, 40, 30} {
		m.throughput.add(now.Add(time.Duration(index-3)*3*time.Second), throughputValue(value))
	}
	return m
}

func containsBraille(value string) bool {
	return strings.ContainsFunc(value, func(character rune) bool {
		return character >= '\u2800' && character <= '\u28ff'
	})
}

// Opt-in offline render artifacts; all values are test fixtures, never queried.
func TestDashboardPreview(t *testing.T) {
	dir := os.Getenv("DASHBOARD_PREVIEW_DIR")
	if dir == "" {
		dir = os.Getenv("SOVKIT_PREVIEW_DIR")
	}
	if dir == "" {
		t.Skip("set DASHBOARD_PREVIEW_DIR to export offline layouts")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, size := range [][2]int{{80, 24}, {120, 36}, {160, 48}} {
		m := NewEndpoint(context.Background(), endpointFixture(), 42, Dependencies{}).SetHealthy(true)
		m.width, m.height = size[0], size[1]
		m = m.WithSession(SessionInfo{Provider: "fixture-cloud", GPU: "Example GPU", Region: "Example region", StartedAt: time.Date(2026, 9, 4, 10, 15, 0, 0, time.UTC)})
		m.events = []string{"10:15:00  SSH tunnel connected", "10:15:01  Model discovery verified"}
		for _, page := range []string{"main", "main-metrics", "generic", "confirm", "ready", "error"} {
			m.page = page
			m.stats = endpointstats.Snapshot{}
			if page == "main-metrics" {
				m.page = "main"
				active, queued, decode, prompt, kv := 2.0, 0.0, 61.4, 280.2, 0.42
				m.stats = endpointstats.Snapshot{Active: &active, Queued: &queued, DecodeTokensPerSecond: &decode, PromptTokensPerSecond: &prompt, KVUsageRatio: &kv, CheckedAt: time.Date(2026, 9, 4, 10, 16, 0, 0, time.UTC)}
			}
			m.target.Kind = clientprofile.Pi
			m.target.Path = "/example/profile/models.json"
			m.inspection.Command = "PI_CODING_AGENT_DIR='/example/profile' pi"
			m.inspection.State = clientprofile.Absent
			m.inspection.Detail = "No provider entry found. The dedicated profile can be added."
			m.inspection.Paths = []string{"/example/profile/models.json"}
			m.status = ""
			if page == "error" {
				m.status = "Example: configuration could not be read."
			}
			name := filepath.Join(dir, fmt.Sprintf("dashboard-%s-%dx%d.txt", page, size[0], size[1]))
			if err := os.WriteFile(name, []byte(ansi.Strip(m.View())+"\n"), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestConfirmationCanScrollBackToReviewAfterSelectionAutoScroll(t *testing.T) {
	m := NewEndpoint(context.Background(), endpointFixture(), 42, Dependencies{}).SetHealthy(true)
	m.width, m.height, m.page = 30, 10, "confirm"
	m.inspection.Detail = strings.Repeat("review these paths ", 12)
	if !strings.Contains(ansi.Strip(m.View()), "Cancel") {
		t.Fatal("selected cancel not visible")
	}
	m, _ = key(m, tea.KeyPgUp)
	if !strings.Contains(ansi.Strip(m.View()), "Review integration") {
		t.Fatal("cannot scroll up to review confirmation details")
	}
}

func TestSessionFactsAndEventsAreShownWithoutInventingHardware(t *testing.T) {
	m := NewEndpoint(context.Background(), endpointFixture(), 42, Dependencies{}).SetHealthy(true)
	m.width, m.height = 120, 40
	m = m.WithSession(SessionInfo{Provider: "test-cloud", GPU: "Known GPU", Region: "Paris", StartedAt: time.Date(2026, 9, 4, 10, 15, 0, 0, time.UTC)})
	next, _ := m.Update(WorkResult{Kind: "discover", Endpoint: endpointFixture()})
	m = next.(Model)
	view := ansi.Strip(m.View())
	for _, want := range []string{"test-cloud", "Known GPU", "Paris", "10:15", "Session events", "Model discovery verified"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing real session fact %q", want)
		}
	}
	if strings.Contains(view, "$0") {
		t.Fatal("missing price is shown as zero")
	}
	next, _ = m.Update(ClipboardResultMsg{Command: "secret-command", Operation: m.operation})
	m = next.(Model)
	if !strings.Contains(ansi.Strip(m.View()), "Copied to clipboard") {
		t.Fatal("clipboard event missing")
	}
	if strings.Contains(ansi.Strip(m.View()), "secret-command") {
		t.Fatal("clipboard payload leaked into events")
	}
}

func TestAllPagesFitViewportAndStripUntrustedControlSequences(t *testing.T) {
	for _, page := range []string{"main", "generic", "inspecting", "confirm", "installing", "ready", "error", "blocked"} {
		for _, width := range []int{12, 30, 80, 120} {
			m := NewEndpoint(context.Background(), endpointFixture(), 42, Dependencies{}).SetHealthy(true)
			m.width, m.height, m.page = width, 12, page
			m.target.Path = strings.Repeat("x", 200)
			m.inspection.Command = "\x1b[31m" + strings.Repeat("x", 200) + "\x07"
			m.status = "\x1b[31m" + strings.Repeat("x", 200) + "\x07"
			view := m.View()
			lines := strings.Split(view, "\n")
			if len(lines) > m.height {
				t.Errorf("%s at %d overflow height: %d", page, width, len(lines))
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > width {
					t.Errorf("%s at %d overflow width: %q", page, width, line)
				}
			}
			if strings.Contains(view, "\x07") || strings.Contains(view, "\x1b[31m") {
				t.Errorf("%s leaked untrusted control code", page)
			}
		}
	}
}

func TestWideDashboardDoesNotPresentMissingThroughputAsZero(t *testing.T) {
	m := NewEndpoint(context.Background(), endpointFixture(), 42, Dependencies{}).SetHealthy(true)
	m.width, m.height = 120, 30
	view := ansi.Strip(m.mainView())
	for _, want := range []string{"Session & instance", "32768", "4096", "not reported", "owner/solo"} {
		if !strings.Contains(view, want) {
			t.Errorf("dashboard missing %q", want)
		}
	}
	if strings.Contains(view, "0 tok/s") {
		t.Fatal("unavailable activity was presented as zero")
	}
}

func TestMeasuredKVUsesBarAndServerSnapshotIsLabelled(t *testing.T) {
	m := NewEndpoint(context.Background(), endpointFixture(), 42, Dependencies{}).SetHealthy(true)
	m.width, m.height = 120, 36
	kv := 0.5
	m.stats = endpointstats.Snapshot{KVUsageRatio: &kv}
	m = m.WithServerLogs("first\nserver ready\n\x1b[31mprivate snapshot\x07", time.Date(2026, 9, 4, 10, 15, 0, 0, time.UTC))
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = next.(Model)
	view := ansi.Strip(m.mainView())
	for _, want := range []string{"50.0%", "█", "Server logs", "last snapshot 10:15", "server ready", "private snapshot"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(view, "\x07") {
		t.Fatal("server log control sequence leaked")
	}
}

func TestCompactNavigationKeepsSelectedActionVisible(t *testing.T) {
	m := NewEndpoint(context.Background(), endpointFixture(), 42, Dependencies{}).SetHealthy(true)
	m.width, m.height = 34, 10
	for i := 0; i < 5; i++ {
		m, _ = key(m, tea.KeyDown)
	}
	if !strings.Contains(ansi.Strip(m.View()), "› OpenCode") {
		t.Fatalf("selected action hidden:\n%s", m.View())
	}
}

func TestGenericConnectionCopiesSelectedRawField(t *testing.T) {
	copied := ""
	m := NewEndpoint(context.Background(), endpointFixture(), 0, Dependencies{Copy: func(s string) error { copied = s; return nil }}).SetHealthy(true)
	m.cursor = 2
	m, _ = key(m, tea.KeyEnter)
	m, cmd := key(m, tea.KeyEnter)
	if cmd == nil {
		t.Fatal("generic connection cannot copy endpoint")
	}
	m = result(m, cmd)
	if copied != "http://127.0.0.1:30000/v1" {
		t.Fatalf("copied %q", copied)
	}
	m, _ = key(m, tea.KeyDown)
	m, cmd = key(m, tea.KeyEnter)
	if cmd == nil {
		t.Fatal("generic connection cannot copy model")
	}
	m = result(m, cmd)
	if copied != "owner/solo" {
		t.Fatalf("copied %q", copied)
	}
}
