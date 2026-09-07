package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestBalanceAppearsRightAlignedOnEveryScreen(t *testing.T) {
	for _, screen := range []string{"home", "resume", "replace", "prompt", "working", "connecting", "saved", "error", "destroy-confirm", "destroyed", "dashboard"} {
		t.Run(screen, func(t *testing.T) {
			m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config.toml"), "root", "home", ApplicationDependencies{})
			defer m.cleanup()
			m.width, m.height, m.screen = 80, 24, screen
			amount := 42.129
			m.vastBalance = &amount
			if screen == "error" {
				m.errText = "fixture failure"
			}
			line := strings.Split(ansi.Strip(m.View()), "\n")[0]
			if !strings.HasSuffix(line, "VAST  $42.13") || ansi.StringWidth(line) != 80 {
				t.Fatalf("balance is not right-aligned: %q", line)
			}
		})
	}
}

func TestApplicationBeginReadsBalanceFromSavedCredential(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	store := FileVastCredentials{Path: path + ".vast-api-key"}
	if err := store.Save("saved-token"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	m := newApplication(context.Background(), path, "root", "home", ApplicationDependencies{
		Setup: SetupDependencies{Getenv: func(string) string { return "" }, Credentials: store},
		Balance: func(_ context.Context, token string) (float64, error) {
			calls++
			if token != "saved-token" {
				t.Fatalf("balance token = %q", token)
			}
			return 8.25, nil
		},
	})
	defer m.cleanup()
	_, command := m.Update(applicationBegin{})
	switch message := command().(type) {
	case vastBalanceResult:
		m.Update(message)
	case tea.BatchMsg:
		for _, scheduled := range message {
			if result, isBalance := scheduled().(vastBalanceResult); isBalance {
				m.Update(result)
			}
		}
	default:
		t.Fatalf("application start scheduled %T", message)
	}
	if calls != 1 || m.vastBalance == nil || *m.vastBalance != 8.25 {
		t.Fatalf("initial balance was not stored: calls=%d balance=%v", calls, m.vastBalance)
	}
}

func TestBalanceAppearsInExpandedLogsHeader(t *testing.T) {
	m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config.toml"), "root", "home", ApplicationDependencies{})
	defer m.cleanup()
	m.width, m.height, m.logsExpanded = 80, 24, true
	amount := 3.5
	m.vastBalance = &amount
	line := strings.Split(ansi.Strip(m.View()), "\n")[0]
	if !strings.HasSuffix(line, "VAST  $3.50") || ansi.StringWidth(line) != 80 {
		t.Fatalf("logs header omits balance: %q", line)
	}
}

func TestBalanceRefreshKeepsLastSuccessfulValue(t *testing.T) {
	m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config.toml"), "root", "home", ApplicationDependencies{})
	defer m.cleanup()
	m.Update(vastBalanceResult{amount: 19.75})
	m.Update(vastBalanceResult{err: errors.New("temporary provider failure")})
	if m.vastBalance == nil || *m.vastBalance != 19.75 {
		t.Fatalf("last successful balance lost: %#v", m.vastBalance)
	}
}

func TestBalanceIsHiddenWithoutCredentialOrSuccessfulRead(t *testing.T) {
	m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config.toml"), "root", "home", ApplicationDependencies{Setup: SetupDependencies{Getenv: func(string) string { return "" }}})
	defer m.cleanup()
	if command := m.readVastBalance(); command != nil {
		t.Fatal("missing credential scheduled a provider request")
	}
	if strings.Contains(ansi.Strip(m.View()), "VAST  $") {
		t.Fatal("missing balance rendered a fallback amount")
	}
}

func TestBalanceHeaderDoesNotWrapAtNarrowWidths(t *testing.T) {
	m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config.toml"), "root", "home", ApplicationDependencies{})
	defer m.cleanup()
	m.width, m.height, m.screen = 30, 12, "working"
	amount := 1234.5
	m.vastBalance = &amount
	line := strings.Split(ansi.Strip(m.View()), "\n")[0]
	if ansi.StringWidth(line) != 30 || !strings.HasSuffix(line, "VAST  $1234.50") {
		t.Fatalf("narrow header wrapped or lost balance: %q", line)
	}
}

func TestBalanceCredentialDisappearanceClearsValueAndKeepsPolling(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	store := FileVastCredentials{Path: path + ".vast-api-key"}
	if err := store.Save("saved-token"); err != nil {
		t.Fatal(err)
	}
	m := newApplication(context.Background(), path, "root", "home", ApplicationDependencies{Setup: SetupDependencies{Getenv: func(string) string { return "" }, Credentials: store}})
	defer m.cleanup()
	amount := 4.5
	m.vastBalance = &amount
	m.balanceStarted = true
	if err := os.Remove(store.Path); err != nil {
		t.Fatal(err)
	}
	command := m.readVastBalance()
	if command == nil {
		t.Fatal("credential disappearance stopped future refreshes")
	}
	result := command().(vastBalanceResult)
	_, next := m.Update(result)
	if m.vastBalance != nil || next == nil {
		t.Fatalf("missing credential retained balance or stopped polling: balance=%v command=%v", m.vastBalance, next)
	}
}

func TestBalanceReadIsCancelledByCleanup(t *testing.T) {
	started := make(chan struct{})
	finished := make(chan tea.Msg, 1)
	m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config.toml"), "root", "home", ApplicationDependencies{
		Setup: SetupDependencies{Getenv: func(key string) string {
			if key == "VAST_API_KEY" {
				return "token"
			}
			return ""
		}},
		Balance: func(ctx context.Context, _ string) (float64, error) {
			close(started)
			<-ctx.Done()
			return 0, ctx.Err()
		},
	})
	go func() { finished <- m.readVastBalance()() }()
	<-started
	m.cleanup()
	select {
	case <-finished:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("cleanup left balance request running")
	}
}
