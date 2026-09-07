package dashboardui

import (
	"testing"
	"time"
)

func throughputValue(value float64) *float64 { return &value }

func TestThroughputHistoryBoundsAndPrunesExpiredSamples(t *testing.T) {
	start := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	var history throughputHistory
	for index := 0; index < 205; index++ {
		history.add(start.Add(time.Duration(index)*3*time.Second), throughputValue(float64(index)))
	}

	samples := history.snapshot(start.Add(205*3*time.Second + throughputWindow + time.Second))
	if got, want := len(samples), 0; got != want {
		t.Fatalf("snapshot length after expiration = %d, want %d", got, want)
	}

	history.reset()
	for index := 0; index < 205; index++ {
		history.add(start.Add(time.Duration(index)*3*time.Second), throughputValue(float64(index)))
	}
	samples = history.snapshot(start.Add(10*time.Minute + 12*time.Second))
	if got, want := len(samples), 200; got != want {
		t.Fatalf("snapshot length = %d, want %d", got, want)
	}
	if got, want := samples[0].At, start.Add(15*time.Second); !got.Equal(want) {
		t.Fatalf("oldest retained sample = %s, want %s", got, want)
	}
}

func TestThroughputHistoryCopiesValuesAndReset(t *testing.T) {
	var history throughputHistory
	at := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	value := 12.5
	history.add(at, &value)
	value = 99
	samples := history.snapshot(at)
	if len(samples) != 1 || samples[0].Generation == nil || *samples[0].Generation != 12.5 {
		t.Fatalf("history did not copy inserted value: %#v", samples)
	}
	*samples[0].Generation = 3
	if got := *history.snapshot(at)[0].Generation; got != 12.5 {
		t.Fatalf("snapshot exposed mutable history value: %v", got)
	}
	history.reset()
	if got := history.snapshot(at); len(got) != 0 {
		t.Fatalf("history after reset has %d samples", len(got))
	}
}

func TestThroughputHistorySummarySemantics(t *testing.T) {
	at := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	var history throughputHistory
	history.add(at, throughputValue(0))
	history.add(at.Add(time.Second), nil)
	history.add(at.Add(2*time.Second), throughputValue(10))
	history.add(at.Add(3*time.Second), throughputValue(20))
	summary := history.summary(at.Add(3 * time.Second))
	if summary.Current == nil || *summary.Current != 20 {
		t.Fatalf("current = %v, want 20", summary.Current)
	}
	if summary.Average == nil || *summary.Average != 10 {
		t.Fatalf("average = %v, want 10", summary.Average)
	}
	if summary.Peak == nil || *summary.Peak != 20 {
		t.Fatalf("peak = %v, want 20", summary.Peak)
	}

	history.add(at.Add(4*time.Second), nil)
	summary = history.summary(at.Add(4 * time.Second))
	if summary.Current != nil {
		t.Fatalf("current after missing newest sample = %v, want nil", *summary.Current)
	}
	if summary.Average == nil || *summary.Average != 10 || summary.Peak == nil || *summary.Peak != 20 {
		t.Fatalf("historical summary erased by missing newest sample: %#v", summary)
	}
}
