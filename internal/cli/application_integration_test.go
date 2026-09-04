package cli

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
)

func TestApplicationManualSaveContinuesIntoOwnedConnection(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"key", "hosts"} {
		if err := os.WriteFile(dir+"/"+name, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	tunnel := &applicationTunnel{done: make(chan error, 1)}
	connections := 0
	m := newApplication(context.Background(), dir+"/config.toml", "alice", "home", ApplicationDependencies{Start: StartDependencies{
		NewTunnel: func(_ context.Context, cfg config.Config, _ io.Writer) (Tunnel, error) {
			connections++
			if cfg.SSH.User != "edited-user" {
				t.Fatalf("saved route not reused: %+v", cfg.SSH)
			}
			return tunnel, nil
		},
		Healthcheck: func(context.Context, string) error { return nil },
	}})
	defer m.cleanup()
	m.Update(m.Init()())
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nextApplicationPrompt(t, m, promptProvider)
	m.acceptPrompt("manual")
	nextApplicationPrompt(t, m, promptManual)
	m.acceptPrompt(ManualRoute{Host: "gpu.example", Port: 22, User: "edited-user", IdentityFile: dir + "/key", KnownHostsFile: dir + "/hosts"})
	for m.session != nil {
		nextApplicationEvent(t, m)
	}
	if m.screen != "saved" {
		t.Fatalf("save failed: %s", m.View())
	}
	_, connect := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if connect == nil {
		t.Fatal("saved continuation did not connect")
	}
	updateApplicationCommand(t, m, connect)
	if m.screen != "dashboard" || connections != 1 {
		t.Fatalf("connection transitions=%d screen=%s", connections, m.screen)
	}
	_, disconnect := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	updateApplicationCommand(t, m, disconnect)
	if m.screen != "home" || tunnel.stops.Load() != 1 {
		t.Fatal("disconnect did not return home and release local tunnel")
	}
	if _, err := config.Load(dir + "/config.toml"); err != nil {
		t.Fatal("disconnect removed saved route")
	}
}

func TestApplicationPaidSetupConnectAndExitKeepRemoteConfiguration(t *testing.T) {
	api := &applicationAPI{}
	m := vastApplication(t, api)
	tunnel := &applicationTunnel{done: make(chan error, 1)}
	m.deps.Start = StartDependencies{NewTunnel: func(context.Context, config.Config, io.Writer) (Tunnel, error) { return tunnel, nil }, Healthcheck: func(context.Context, string) error { return nil }}
	advanceVastToCost(t, m)
	m.acceptPrompt(true)
	m.acceptPrompt(true) // repeated submit has no pending request
	nextApplicationPrompt(t, m, promptHostKeys)
	m.acceptPrompt(true)
	for m.session != nil {
		nextApplicationEvent(t, m)
	}
	if m.screen != "saved" || api.creates.Load() != 1 {
		t.Fatalf("paid setup: creates=%d screen=%s", api.creates.Load(), m.screen)
	}
	before, err := os.ReadFile(m.path)
	if err != nil {
		t.Fatal(err)
	}
	_, connect := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updateApplicationCommand(t, m, connect)
	if m.screen != "dashboard" {
		t.Fatalf("paid route did not connect: %s", m.View())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !strings.Contains(m.View(), "billing") {
		t.Fatal("paid exit omits billing warning")
	}
	_, quit := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if quit != nil {
		m.Update(quit())
	}
	m.cleanup()
	after, err := os.ReadFile(m.path)
	if err != nil || string(before) != string(after) {
		t.Fatal("exit changed remote route configuration")
	}
	if api.creates.Load() != 1 || tunnel.stops.Load() != 1 || m.instanceID != 987 {
		t.Fatal("exit violated local-only cleanup or lost paid instance identity")
	}
}
