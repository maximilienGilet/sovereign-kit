package dashboardui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/maximilienGilet/sovereign-kit/internal/clientprofile"
)

func key(m Model, kind tea.KeyType) (Model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyMsg{Type: kind})
	return next.(Model), cmd
}
func result(m Model, cmd tea.Cmd) Model { next, _ := m.Update(cmd()); return next.(Model) }
func endpointFixture() clientprofile.Endpoint {
	return clientprofile.Endpoint{BaseURL: "http://127.0.0.1:30000/v1", Metadata: clientprofile.Metadata{ID: "owner/solo", ContextWindow: 32768, MaxTokens: 4096}}
}

func TestSavedMetadataIsNotPresentedAsDiscoveredIdentity(t *testing.T) {
	e := endpointFixture()
	e.Problem = "Checking model identity…"
	m := NewEndpoint(context.Background(), e, 0, Dependencies{}).SetHealthy(true)
	if strings.Contains(m.View(), "owner/solo") {
		t.Fatal("saved model presented before discovery")
	}
}

func TestInitialDiscoverySurvivesOpeningIntegration(t *testing.T) {
	e := endpointFixture()
	e.Problem = "Checking model identity…"
	m := NewEndpoint(context.Background(), e, 0, Dependencies{Discover: func(context.Context, string, clientprofile.Metadata) clientprofile.Endpoint { return endpointFixture() }, Resolve: func(k clientprofile.Integration) (clientprofile.Target, error) {
		return clientprofile.Target{k, "/tmp/fixture"}, nil
	}, Inspect: func(context.Context, clientprofile.Target, clientprofile.Endpoint) clientprofile.Inspection {
		return clientprofile.Inspection{State: clientprofile.Absent}
	}}).SetHealthy(true)
	discovery := m.Init()
	m.cursor = 3
	m, inspect := key(m, tea.KeyEnter)
	m = result(m, inspect)
	m = result(m, discovery)
	m, _ = key(m, tea.KeyEsc)
	if !strings.Contains(m.View(), "owner/solo") || strings.Contains(m.View(), "Checking model identity") {
		t.Fatal("integration discarded initial model discovery")
	}
	m, inspect = key(m, tea.KeyEnter)
	m = result(m, inspect)
	if m.page != "confirm" {
		t.Fatalf("discovered model did not enable fresh inspection: %s", m.page)
	}
}
func TestIntegrationInspectsBeforeCancelDefaultConfirmation(t *testing.T) {
	inspections, installs := 0, 0
	m := NewEndpoint(context.Background(), endpointFixture(), 42, Dependencies{
		Resolve: func(k clientprofile.Integration) (clientprofile.Target, error) {
			return clientprofile.Target{Kind: k, Path: "/temporary/agent"}, nil
		},
		Inspect: func(context.Context, clientprofile.Target, clientprofile.Endpoint) clientprofile.Inspection {
			inspections++
			return clientprofile.Inspection{State: clientprofile.Absent, Paths: []string{"/temporary/agent/settings.json"}}
		},
		Install: func(context.Context, clientprofile.Target, clientprofile.Endpoint, clientprofile.Inspection) (clientprofile.Inspection, error) {
			installs++
			return clientprofile.Inspection{State: clientprofile.Ready, Command: "manual command"}, nil
		},
	}).SetHealthy(true)
	m.cursor = 3 // endpoint, model, generic instructions, Pi
	m, cmd := key(m, tea.KeyEnter)
	if inspections != 0 || installs != 0 || cmd == nil {
		t.Fatal("work ran inside Update or missing inspection")
	}
	m = result(m, cmd)
	if inspections != 1 || installs != 0 || !strings.Contains(m.View(), "Cancel") {
		t.Fatal("confirmation missing")
	}
	m, cmd = key(m, tea.KeyEnter)
	if cmd != nil || installs != 0 || m.page != "main" {
		t.Fatal("default Enter installed")
	}
	m, cmd = key(m, tea.KeyEnter)
	m = result(m, cmd)
	m, _ = key(m, tea.KeyRight)
	m, cmd = key(m, tea.KeyEnter)
	if installs != 0 || cmd == nil {
		t.Fatal("install must be asynchronous after confirmation")
	}
	m = result(m, cmd)
	if installs != 1 || !strings.Contains(m.View(), "manual command") {
		t.Fatal("verified command not shown")
	}
}
func TestReadyBypassesInstallAndCopiesRawValuesWithFeedback(t *testing.T) {
	for _, copyErr := range []error{nil, errors.New("clipboard unavailable")} {
		copied := ""
		m := NewEndpoint(context.Background(), endpointFixture(), 0, Dependencies{Copy: func(v string) error { copied = v; return copyErr }, Resolve: func(k clientprofile.Integration) (clientprofile.Target, error) {
			return clientprofile.Target{k, "/tmp/profile"}, nil
		}, Inspect: func(context.Context, clientprofile.Target, clientprofile.Endpoint) clientprofile.Inspection {
			return clientprofile.Inspection{State: clientprofile.Ready, Command: "PI_CODING_AGENT_DIR='/tmp/profile' pi"}
		}, Install: func(context.Context, clientprofile.Target, clientprofile.Endpoint, clientprofile.Inspection) (clientprofile.Inspection, error) {
			t.Fatal("ready installed")
			return clientprofile.Inspection{}, nil
		}}).SetHealthy(true)
		m, cmd := key(m, tea.KeyEnter)
		m = result(m, cmd)
		if copied != "http://127.0.0.1:30000/v1" {
			t.Fatalf("raw endpoint %q", copied)
		}
		if copyErr != nil && !strings.Contains(m.View(), "Copy failed") {
			t.Fatal("copy failure hidden")
		}
		m.cursor = 3
		m, cmd = key(m, tea.KeyEnter)
		m = result(m, cmd)
		m, cmd = key(m, tea.KeyEnter)
		m = result(m, cmd)
		if copied != "PI_CODING_AGENT_DIR='/tmp/profile' pi" {
			t.Fatal("ready command not copied")
		}
	}
}
func TestInstallFailureAndStaleResultsNeverUnlockCommand(t *testing.T) {
	m := NewEndpoint(context.Background(), endpointFixture(), 0, Dependencies{}).SetHealthy(true)
	m.page = "installing"
	m.operation = 2
	m.busy = true
	next, _ := m.Update(WorkResult{Operation: 1, Kind: "install", Inspection: clientprofile.Inspection{State: clientprofile.Ready, Command: "unsafe"}})
	m = next.(Model)
	if strings.Contains(m.View(), "unsafe") {
		t.Fatal("stale install revived")
	}
	next, _ = m.Update(WorkResult{Operation: 2, Kind: "install", Inspection: clientprofile.Inspection{State: clientprofile.Ready, Command: "unsafe"}, Err: errors.New("failed package")})
	m = next.(Model)
	if strings.Contains(m.View(), "unsafe") || !strings.Contains(m.View(), "failed package") {
		t.Fatal("failure unlocked ready")
	}
	m, _ = key(m, tea.KeyEsc)
	if m.page != "main" {
		t.Fatal("escape did not stay in dashboard")
	}
}
func TestNarrowDashboardScrollsAndMissingMetadataBlocksConfirmation(t *testing.T) {
	m := NewEndpoint(context.Background(), clientprofile.Endpoint{BaseURL: "http://127.0.0.1:30000/v1", LimitsProblem: "Limits missing"}, 0, Dependencies{Resolve: func(k clientprofile.Integration) (clientprofile.Target, error) {
		return clientprofile.Target{k, "/tmp/fixture"}, nil
	}, Inspect: func(context.Context, clientprofile.Target, clientprofile.Endpoint) clientprofile.Inspection {
		return clientprofile.Inspection{State: clientprofile.Absent}
	}}).SetHealthy(true)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 30, Height: 8})
	m = next.(Model)
	m.cursor = 3
	m, cmd := key(m, tea.KeyEnter)
	if cmd == nil {
		t.Fatal("missing metadata skipped read-only inspection")
	}
	m = result(m, cmd)
	if !strings.Contains(m.View(), "unavailable") {
		t.Fatal("unavailable reason hidden")
	}
	m, _ = key(m, tea.KeyPgDown)
	if len(strings.Split(m.View(), "\n")) > 8 {
		t.Fatal("small layout overflow")
	}
}
