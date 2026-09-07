package dashboardui

import (
	"context"
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/maximilienGilet/sovereign-kit/internal/clientprofile"
	"github.com/maximilienGilet/sovereign-kit/internal/endpointstats"
)

func TestDashboardThroughputHistoryRecordsTimestampedAndMissingSamples(t *testing.T) {
	start := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	first, second := 12.5, 19.75
	m := NewEndpoint(context.Background(), endpointFixture(), 0, Dependencies{}).SetHealthy(true)

	m = updateDashboardStats(t, m, endpointstats.Snapshot{CheckedAt: start, DecodeTokensPerSecond: &first})
	m = updateDashboardStats(t, m, endpointstats.Snapshot{CheckedAt: start.Add(3 * time.Second), DecodeTokensPerSecond: &second})
	m = updateDashboardStats(t, m, endpointstats.Snapshot{CheckedAt: start.Add(6 * time.Second)})

	samples := m.throughput.snapshot(start.Add(6 * time.Second))
	if got, want := len(samples), 3; got != want {
		t.Fatalf("sample count = %d, want %d", got, want)
	}
	for index, want := range []struct {
		at    time.Time
		value *float64
	}{
		{start, &first},
		{start.Add(3 * time.Second), &second},
		{start.Add(6 * time.Second), nil},
	} {
		if !samples[index].At.Equal(want.at) {
			t.Errorf("sample %d timestamp = %s, want %s", index, samples[index].At, want.at)
		}
		if want.value == nil {
			if samples[index].Generation != nil {
				t.Errorf("sample %d generation = %v, want missing", index, *samples[index].Generation)
			}
			continue
		}
		if samples[index].Generation == nil || *samples[index].Generation != *want.value {
			t.Errorf("sample %d generation = %v, want %v", index, samples[index].Generation, *want.value)
		}
	}

	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	samples = m.throughput.snapshot(start.Add(6 * time.Second))
	if got, want := len(samples), 3; got != want {
		t.Fatalf("resize discarded samples: got %d, want %d", got, want)
	}
}

func TestDashboardThroughputHistoryUsesClockForUntimestampedSuccessfulStats(t *testing.T) {
	value := 7.25
	m := NewEndpoint(context.Background(), endpointFixture(), 0, Dependencies{}).SetHealthy(true)
	before := time.Now()
	m = updateDashboardStats(t, m, endpointstats.Snapshot{DecodeTokensPerSecond: &value})
	after := time.Now()

	samples := m.throughput.snapshot(after)
	if got, want := len(samples), 1; got != want {
		t.Fatalf("sample count = %d, want %d", got, want)
	}
	if samples[0].At.Before(before) || samples[0].At.After(after) {
		t.Fatalf("untimestamped sample timestamp = %s, want model clock between %s and %s", samples[0].At, before, after)
	}
	if samples[0].Generation == nil || *samples[0].Generation != value {
		t.Fatalf("untimestamped sample generation = %v, want %v", samples[0].Generation, value)
	}
}

func TestDashboardThroughputHistoryPreservesTelemetryGapsAndFrozenDisconnect(t *testing.T) {
	start := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	value, ignored := 8.0, 99.0
	m := NewEndpoint(context.Background(), endpointFixture(), 0, Dependencies{}).SetHealthy(true)
	m = updateDashboardStats(t, m, endpointstats.Snapshot{CheckedAt: start, DecodeTokensPerSecond: &value})
	m = updateDashboardStats(t, m, endpointstats.Snapshot{CheckedAt: start.Add(3 * time.Second), Problem: "metrics unavailable"})

	samples := m.throughput.snapshot(start.Add(3 * time.Second))
	if got, want := len(samples), 2; got != want || samples[0].Generation == nil || *samples[0].Generation != value || samples[1].Generation != nil {
		t.Fatalf("timestamped telemetry interruption lost history or gap: %#v", samples)
	}

	m = m.SetHealthy(false)
	m = updateDashboardStats(t, m, endpointstats.Snapshot{CheckedAt: start.Add(6 * time.Second), DecodeTokensPerSecond: &ignored})
	samples = m.throughput.snapshot(start.Add(6 * time.Second))
	if got, want := len(samples), 2; got != want {
		t.Fatalf("disconnected stats added a sample: got %d, want %d", got, want)
	}
}

func TestDashboardThroughputHistoryResetsForReconnectionAndEndpointIdentityChange(t *testing.T) {
	start := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	oldValue, newValue := 6.0, 18.0
	m := NewEndpoint(context.Background(), endpointFixture(), 0, Dependencies{}).SetHealthy(true)
	m = updateDashboardStats(t, m, endpointstats.Snapshot{CheckedAt: start, DecodeTokensPerSecond: &oldValue})

	m = m.SetHealthy(false).SetHealthy(true)
	if got := len(m.throughput.snapshot(start)); got != 0 {
		t.Fatalf("reconnection retained %d samples", got)
	}
	m = updateDashboardStats(t, m, endpointstats.Snapshot{CheckedAt: start.Add(3 * time.Second), DecodeTokensPerSecond: &newValue})
	if samples := m.throughput.snapshot(start.Add(3 * time.Second)); len(samples) != 1 || samples[0].Generation == nil || *samples[0].Generation != newValue {
		t.Fatalf("reconnection did not begin a fresh history: %#v", samples)
	}

	next, _ := m.Update(WorkResult{Kind: "discover", Endpoint: clientprofile.Endpoint{
		BaseURL:  "http://127.0.0.1:30000/v1",
		Metadata: clientprofile.Metadata{ID: "owner/replaced", ContextWindow: 32768, MaxTokens: 4096},
	}})
	m = next.(Model)
	if got := len(m.throughput.snapshot(start.Add(3 * time.Second))); got != 0 {
		t.Fatalf("new endpoint identity retained %d samples", got)
	}
}

func TestDashboardThroughputHistoryIgnoresUntimestampedTelemetryFailure(t *testing.T) {
	start := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	value := 4.0
	m := NewEndpoint(context.Background(), endpointFixture(), 0, Dependencies{}).SetHealthy(true)
	m = updateDashboardStats(t, m, endpointstats.Snapshot{CheckedAt: start, DecodeTokensPerSecond: &value})
	m = updateDashboardStats(t, m, endpointstats.Snapshot{Problem: "query failed"})

	if samples := m.throughput.snapshot(start); len(samples) != 1 || samples[0].Generation == nil || *samples[0].Generation != value {
		t.Fatalf("untimestamped failure changed retained history: %#v", samples)
	}
}

func updateDashboardStats(t *testing.T, m Model, snapshot endpointstats.Snapshot) Model {
	t.Helper()
	next, _ := m.Update(statsResultMsg{snapshot: snapshot, epoch: m.statsEpoch})
	return next.(Model)
}

func TestDashboardRejectsStatsReadStartedBeforeReconnect(t *testing.T) {
	value := 91.0
	m := NewEndpoint(context.Background(), endpointFixture(), 42, Dependencies{
		Stats: func(context.Context, string) endpointstats.Snapshot {
			return endpointstats.Snapshot{CheckedAt: time.Now(), DecodeTokensPerSecond: &value, Active: &value}
		},
	}).SetHealthy(true)
	oldRead := m.readStats()
	m = m.SetHealthy(false).SetHealthy(true)
	next, wait := m.Update(oldRead())
	m = next.(Model)
	if m.stats.Active != nil || m.stats.DecodeTokensPerSecond != nil || len(m.throughput.snapshot(time.Now())) != 0 {
		t.Fatal("late read from previous tunnel repopulated the new session")
	}
	if wait == nil {
		t.Fatal("discarding a stale result stopped the chained polling loop")
	}
	next, read := m.Update(statsTickMsg{})
	m = next.(Model)
	next, wait = m.Update(read())
	m = next.(Model)
	if m.stats.Active == nil || *m.stats.Active != value || len(m.throughput.snapshot(time.Now())) != 1 || wait == nil {
		t.Fatal("fresh session read did not resume polling and measurements")
	}
}

func TestDashboardIdentityChangeAcrossDiscoveryFailureInvalidatesCurrentAndPendingStats(t *testing.T) {
	for _, failedDiscovery := range []bool{false, true} {
		t.Run(fmt.Sprint(failedDiscovery), func(t *testing.T) {
			value := 83.0
			m := NewEndpoint(context.Background(), endpointFixture(), 42, Dependencies{
				Stats: func(context.Context, string) endpointstats.Snapshot {
					return endpointstats.Snapshot{CheckedAt: time.Now(), DecodeTokensPerSecond: &value, Active: &value}
				},
			}).SetHealthy(true)
			m.statsStarted = true
			next, _ := m.Update(m.readStats()())
			m = next.(Model)
			oldRead := m.readStats()
			if failedDiscovery {
				next, _ = m.Update(WorkResult{Kind: "discover", Endpoint: clientprofile.Endpoint{BaseURL: m.endpoint.BaseURL, Problem: "discovery unavailable"}})
				m = next.(Model)
			}
			replacement := endpointFixture()
			replacement.ID = "owner/replacement"
			next, duplicate := m.Update(WorkResult{Kind: "discover", Endpoint: replacement})
			m = next.(Model)
			if duplicate != nil {
				t.Fatal("identity change started an overlapping polling chain")
			}
			if m.stats.Active != nil || m.stats.DecodeTokensPerSecond != nil || len(m.throughput.snapshot(time.Now())) != 0 {
				t.Fatal("replacement model retained previous measurements")
			}
			next, wait := m.Update(oldRead())
			m = next.(Model)
			if m.stats.Active != nil || len(m.throughput.snapshot(time.Now())) != 0 || wait == nil {
				t.Fatal("old model read repopulated replacement model or stopped polling")
			}
		})
	}
}
