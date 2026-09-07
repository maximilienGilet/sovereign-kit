package dashboardui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestThroughputCeilingUsesRoundedOneTwoFiveSteps(t *testing.T) {
	tests := []struct {
		name    string
		values  []float64
		ceiling float64
	}{
		{name: "empty", ceiling: 1},
		{name: "zero", values: []float64{0}, ceiling: 1},
		{name: "one", values: []float64{1}, ceiling: 1},
		{name: "two", values: []float64{1.01}, ceiling: 2},
		{name: "five", values: []float64{2.01}, ceiling: 5},
		{name: "next decade", values: []float64{5.01}, ceiling: 10},
		{name: "fractional", values: []float64{0.21}, ceiling: 0.5},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			samples := make([]throughputSample, 0, len(test.values)+1)
			samples = append(samples, throughputSample{}) // Missing reports never affect the scale.
			for _, value := range test.values {
				value := value
				samples = append(samples, throughputSample{Generation: &value})
			}
			if got := throughputCeiling(samples); got != test.ceiling {
				t.Fatalf("throughputCeiling(%v) = %g, want %g", test.values, got, test.ceiling)
			}
		})
	}
}

func TestThroughputCeilingKeepsPeakWhileItRemainsInWindow(t *testing.T) {
	peak, current := 73.0, 12.0
	samples := []throughputSample{{Generation: &peak}, {Generation: &current}}
	if got := throughputCeiling(samples); got != 100 {
		t.Fatalf("ceiling with retained peak = %g, want 100", got)
	}
}

func TestThroughputChartHasFixedGeometryAndAxisLabels(t *testing.T) {
	now := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	first, middle, last := 0.0, 25.0, 50.0
	samples := []throughputSample{
		{At: now.Add(-throughputWindow), Generation: &first},
		{At: now.Add(-throughputWindow / 2), Generation: &middle},
		{At: now, Generation: &last},
	}

	rendered := ansi.Strip(renderThroughputChart(samples, now, 42, 8, false))
	assertChartGeometry(t, rendered, 42, 8)
	for _, label := range []string{"-10m", "-5m", "now", "0"} {
		if !strings.Contains(rendered, label) {
			t.Errorf("chart is missing axis label %q:\n%s", label, rendered)
		}
	}
}

func TestThroughputChartDoesNotBridgeMissingSamples(t *testing.T) {
	now := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	value := 4.0
	missing := []throughputSample{
		{At: now.Add(-8 * time.Minute), Generation: &value},
		{At: now.Add(-5 * time.Minute), Generation: nil},
		{At: now.Add(-2 * time.Minute), Generation: &value},
	}
	continuous := []throughputSample{missing[0], missing[2]}

	brokenRender := ansi.Strip(renderThroughputChart(missing, now, 38, 7, false))
	continuousRender := ansi.Strip(renderThroughputChart(continuous, now, 38, 7, false))
	if got := countBrailleRunes(brokenRender); got != 2 {
		t.Fatalf("missing report produced %d plotted cells, want two isolated points:\n%s", got, brokenRender)
	}
	if got, broken := countBrailleRunes(continuousRender), countBrailleRunes(brokenRender); got <= broken {
		t.Fatalf("adjacent reports were not joined: connected cells %d, broken cells %d", got, broken)
	}
}

func TestThroughputChartGeometryDoesNotDependOnValues(t *testing.T) {
	now := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	lowA, lowB, highA, highB := 1.0, 2.0, 90.0, 5.0
	low := renderThroughputChart([]throughputSample{
		{At: now.Add(-9 * time.Minute), Generation: &lowA},
		{At: now.Add(-time.Minute), Generation: &lowB},
	}, now, 33, 6, false)
	high := renderThroughputChart([]throughputSample{
		{At: now.Add(-9 * time.Minute), Generation: &highA},
		{At: now.Add(-time.Minute), Generation: &highB},
	}, now, 33, 6, false)

	assertChartGeometry(t, ansi.Strip(low), 33, 6)
	assertChartGeometry(t, ansi.Strip(high), 33, 6)
}

func TestThroughputChartShowsZeroBaselineAndUnavailableWindow(t *testing.T) {
	now := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	zero := 0.0
	zeroRender := ansi.Strip(renderThroughputChart([]throughputSample{
		{At: now.Add(-throughputWindow), Generation: &zero},
		{At: now, Generation: &zero},
	}, now, 36, 7, false))
	if countBrailleRunes(zeroRender) == 0 {
		t.Fatalf("reported zero window has no visible baseline:\n%s", zeroRender)
	}

	unavailable := ansi.Strip(renderThroughputChart([]throughputSample{
		{At: now.Add(-time.Minute), Generation: nil},
	}, now, 36, 7, false))
	if !strings.Contains(unavailable, "Throughput unavailable") {
		t.Fatalf("missing window has no explicit unavailable state:\n%s", unavailable)
	}
}

func TestThroughputChartKeepsMutedHistoryWhenInterrupted(t *testing.T) {
	now := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	first, last := 5.0, 12.0
	rendered := renderThroughputChart([]throughputSample{
		{At: now.Add(-8 * time.Minute), Generation: &first},
		{At: now.Add(-2 * time.Minute), Generation: &last},
	}, now, 40, 8, true)
	plain := ansi.Strip(rendered)
	if !strings.Contains(plain, "Telemetry interrupted") {
		t.Fatalf("interruption is not identified:\n%s", plain)
	}
	if countBrailleRunes(plain) == 0 {
		t.Fatalf("interruption erased recorded history:\n%s", plain)
	}
	if !strings.Contains(rendered, muted.Render("Telemetry interrupted")) {
		t.Fatalf("interruption notice does not use the muted palette: %q", rendered)
	}
}

func TestThroughputChartSanitizesInvalidDimensions(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("invalid dimensions caused a panic: %v", recovered)
		}
	}()
	_ = renderThroughputChart(nil, time.Time{}, -20, -4, false)
}

func assertChartGeometry(t *testing.T, rendered string, width, height int) {
	t.Helper()
	lines := strings.Split(rendered, "\n")
	if got := len(lines); got != height {
		t.Fatalf("chart height = %d, want %d:\n%s", got, height, rendered)
	}
	for index, line := range lines {
		if got := ansi.StringWidth(line); got != width {
			t.Fatalf("chart line %d width = %d, want %d: %q", index, got, width, line)
		}
	}
}

func countBrailleRunes(value string) int {
	count := 0
	for _, r := range value {
		if r >= '\u2800' && r <= '\u28ff' {
			count++
		}
	}
	return count
}
