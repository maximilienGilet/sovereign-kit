package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
)

// These tests exercise Setup's actual file write through the root's worker bridge.
func TestApplicationManualSetupAndReplacementRefusal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	key := filepath.Join(dir, "key")
	hosts := filepath.Join(dir, "hosts")
	for _, file := range []string{key, hosts} {
		if err := os.WriteFile(file, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m := newApplication(context.Background(), path, "root", "setup", ApplicationDependencies{})
	defer m.cleanup()
	m.Update(m.Init()())
	nextApplicationPrompt(t, m, promptProvider)
	m.acceptPrompt("manual")
	nextApplicationPrompt(t, m, promptManual)
	m.acceptPrompt(ManualRoute{Host: "gpu.example", Port: 22, User: "edited-user", IdentityFile: key, KnownHostsFile: hosts})
	for m.session != nil {
		nextApplicationEvent(t, m)
	}
	cfg, err := config.Load(path)
	if err != nil || cfg.SSH.User != "edited-user" {
		t.Fatalf("manual save: %+v %v", cfg, err)
	}
	if m.screen != "saved" || strings.Contains(m.View(), "LIVE") {
		t.Fatalf("save must not claim health: %s", m.View())
	}

	replacement := newApplication(context.Background(), path, "root", "setup", ApplicationDependencies{})
	defer replacement.cleanup()
	replacement.Update(replacement.Init()())
	if replacement.screen != "replace" || replacement.session != nil {
		t.Fatal("replacement started Setup without consent")
	}
	replacement.Update(tea.KeyMsg{Type: tea.KeyEnter}) // default No
	if replacement.screen != "home" || replacement.session != nil {
		t.Fatal("declining replacement did not return home")
	}
}

func TestApplicationBackRetainsManualDraftAndQIsText(t *testing.T) {
	m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config.toml"), "root", "setup", ApplicationDependencies{})
	defer m.cleanup()
	m.Update(m.Init()())
	nextApplicationPrompt(t, m, promptProvider)
	m.acceptPrompt("manual")
	nextApplicationPrompt(t, m, promptManual)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if m.editor.route.Host != "q" || m.quitting {
		t.Fatal("q did not remain input")
	}
	m.editor.route.User = "retained"
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	nextApplicationPrompt(t, m, promptProvider)
	m.acceptPrompt("manual")
	nextApplicationPrompt(t, m, promptManual)
	if m.editor.route.Host != "q" || m.editor.route.User != "retained" {
		t.Fatalf("draft erased: %+v", m.editor.route)
	}
}

func nextApplicationEvent(t *testing.T, m *applicationModel) {
	t.Helper()
	if m.session == nil {
		t.Fatal("no active session")
	}
	result := make(chan tea.Msg, 1)
	go func() { result <- m.session.next()() }()
	select {
	case msg := <-result:
		m.Update(msg)
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not produce an event")
	}
}

func nextApplicationPrompt(t *testing.T, m *applicationModel, kind promptKind) {
	t.Helper()
	for i := 0; i < 30; i++ {
		if m.pending != nil {
			if m.pending.kind != kind {
				t.Fatalf("prompt %s want %s", m.pending.kind, kind)
			}
			return
		}
		nextApplicationEvent(t, m)
	}
	t.Fatal("prompt did not arrive")
}

func TestApplicationSmallTerminalCanConfirmExit(t *testing.T) {
	m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config.toml"), "root", "home", ApplicationDependencies{})
	m.Update(tea.WindowSizeMsg{Width: 29, Height: 9})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !strings.Contains(m.View(), "y exit") {
		t.Fatal("small terminal hides exit confirmation")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil || !m.quitting {
		t.Fatal("small terminal cannot quit")
	}
}

func TestApplicationRecipeHasOneHeaderAndScrollableSmallBody(t *testing.T) {
	m := vastApplication(t, &applicationAPI{})
	nextApplicationPrompt(t, m, promptProvider)
	m.acceptPrompt("vast")
	nextApplicationPrompt(t, m, promptWorkload)
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 34})
	if count := strings.Count(m.View(), "SOVEREIGN KIT"); count != 1 {
		t.Fatalf("recipe has %d brand headers", count)
	}
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	before := m.View()
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if before == m.View() {
		t.Fatal("compact recipe content cannot scroll")
	}
}

func TestApplicationInvalidReplacementRefusalNeverInvokesVast(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("not valid TOML env-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	deps := ApplicationDependencies{Setup: SetupDependencies{
		Getenv: func(string) string { return "env-secret" },
		RunVast: func(context.Context, string, string, VastWorkload, setup.Operator) (setup.Result, error) {
			calls++
			return setup.Result{}, nil
		},
	}}
	m := newApplication(context.Background(), path, "root", "setup", deps)
	defer m.cleanup()
	m.Update(m.Init()())
	if m.session != nil || m.screen != "replace" {
		t.Fatal("invalid config bypassed replacement consent")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if calls != 0 || m.session != nil || m.screen != "home" {
		t.Fatal("declining replacement invoked setup")
	}
	if strings.Contains(m.View(), "env-secret") {
		t.Fatal("config diagnostic exposed environment key")
	}
}

func TestApplicationFormFitsSupportedScreens(t *testing.T) {
	for _, size := range [][2]int{{30, 10}, {72, 24}, {150, 34}} {
		m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config.toml"), "root", "setup", ApplicationDependencies{})
		t.Cleanup(m.cleanup)
		m.Update(m.Init()())
		nextApplicationPrompt(t, m, promptProvider)
		m.acceptPrompt("manual")
		nextApplicationPrompt(t, m, promptManual)
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("gpu.example")})
		view := m.View()
		if !strings.Contains(view, "GPU host") || !strings.Contains(view, "gpu.example") || !strings.Contains(view, "Enter next") {
			t.Fatalf("form hides input/controls at %v: %s", size, view)
		}
		if os.Getenv("SOVKIT_CAPTURE_VIEWS") != "" {
			t.Logf("FRAME %dx%d\n%s", size[0], size[1], ansi.Strip(view))
		}
	}
}

func TestApplicationMinimumProviderKeepsFocusedChoiceVisibleAcrossResize(t *testing.T) {
	m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config.toml"), "alice", "setup", ApplicationDependencies{})
	defer m.cleanup()
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	m.Update(m.Init()())
	nextApplicationPrompt(t, m, promptProvider)
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.editor.text != "manual" {
		t.Fatalf("selection=%q", m.editor.text)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "> Manual SSH") {
		t.Fatalf("focused provider hidden:\n%s", view)
	}
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 34})
	if view := ansi.Strip(m.View()); !strings.Contains(view, "Vast creates a paid GPU instance") {
		t.Fatalf("large screen lost provider guidance:\n%s", view)
	}
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	if view := ansi.Strip(m.View()); !strings.Contains(view, "> Manual SSH") || m.editor.text != "manual" {
		t.Fatalf("resize lost focused selection:\n%s", view)
	}
}

func TestApplicationMinimumInputsKeepEditableControlVisible(t *testing.T) {
	for _, kind := range []promptKind{promptQuery, promptToken, promptIdentity} {
		t.Run(string(kind), func(t *testing.T) {
			m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config.toml"), "alice", "home", ApplicationDependencies{})
			defer m.cleanup()
			m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
			request := promptRequest{kind: kind, reply: make(chan any, 1)}
			if kind == promptIdentity {
				request.data = "/key"
			}
			m.openPrompt(request)
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("xyz")})
			view := ansi.Strip(m.View())
			if kind == promptToken {
				if strings.Contains(view, "xyz") || !strings.Contains(view, "***") {
					t.Fatalf("password control hidden/unmasked:\n%s", view)
				}
			} else if !strings.Contains(view, "xyz") {
				t.Fatalf("editable control hidden:\n%s", view)
			}
		})
	}
}
