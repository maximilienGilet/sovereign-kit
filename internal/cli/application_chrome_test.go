package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/clientprofile"
	"github.com/maximilienGilet/sovereign-kit/internal/dashboardui"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/muesli/termenv"
)

func TestErrorShowsCauseBeforeDecoration(t *testing.T) {
	m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config.toml"), "root", "start", ApplicationDependencies{})
	defer m.cleanup()
	m.width, m.height, m.screen = 100, 36, "error"
	m.errText = "SSH fingerprint verification failed"
	view := strings.Split(ansi.Strip(m.View()), "\n")
	found := false
	for _, line := range view[:min(10, len(view))] {
		found = found || strings.Contains(line, m.errText)
	}
	if !found {
		t.Fatal("error cause hidden below decorative cube")
	}
}

func TestApplicationScreenRefreshFitsAndPreview(t *testing.T) {
	for _, size := range [][2]int{{40, 14}, {80, 24}, {120, 36}, {160, 48}} {
		for _, screen := range []string{"home", "resume", "replace", "saved", "error", "destroy-confirm", "destroyed", "working", "exit"} {
			t.Run(fmt.Sprintf("%s-%dx%d", screen, size[0], size[1]), func(t *testing.T) {
				m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config.toml"), "root", "home", ApplicationDependencies{})
				defer m.cleanup()
				m.width, m.height, m.screen = size[0], size[1], screen
				m.status = "Waiting for the instance…"
				if screen == "error" {
					m.errText = "SSH fingerprint verification failed. The existing instance has not been destroyed."
				}
				if screen == "destroyed" {
					m.recoveryText = "Instance operation completed."
				}
				if screen == "working" {
					m.applyProgress(setup.Progress{Stage: setup.ProgressWaiting, InstanceID: 4242})
					m.status = progressLabel(setup.ProgressWaiting)
					m.loader.stage(m.status, time.Now())
				}
				if screen == "exit" {
					m.screen = "dashboard"
					m.exitConfirm = true
					m.exitInstance = 4242
				}
				m.resizeChild()
				view := m.View()
				lines := strings.Split(strings.TrimSuffix(view, "\n"), "\n")
				if len(lines) > size[1] {
					t.Fatalf("height %d > %d", len(lines), size[1])
				}
				for _, line := range lines {
					if ansi.StringWidth(line) > size[0] {
						t.Fatalf("overflow %q", line)
					}
				}
				if dir := os.Getenv("SOVKIT_PREVIEW_DIR"); dir != "" {
					if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-%dx%d.txt", screen, size[0], size[1])), []byte(ansi.Strip(view)), 0600); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestHomePrimaryActionHasVisualHierarchy(t *testing.T) {
	m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config.toml"), "root", "start", ApplicationDependencies{})
	defer m.cleanup()
	m.width, m.height, m.screen = 120, 36, "home"
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "PRIVATE AI, YOUR WAY") || !strings.Contains(view, "[ Enter ]") {
		t.Fatalf("missing primary action panel:\n%s", view)
	}
}

func TestShimmerCompletesSweepWithinTwoSeconds(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	// Same point in a 2.4s cycle must produce the same frame; the former 4.8s
	// animation took twice as long and remained halfway through its sweep.
	label := "Waiting for the private inference server"
	if shimmer(label, 600*time.Millisecond) != shimmer(label, 3*time.Second) {
		t.Fatal("shimmer cycle exceeds requested faster cadence")
	}
	if ansi.Strip(shimmer(label, 600*time.Millisecond)) != label {
		t.Fatal("shimmer changed content")
	}
}

func TestEmbeddedDashboardPreview(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 36}, {160, 48}} {
		m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config.toml"), "root", "home", ApplicationDependencies{})
		m.width, m.height, m.screen = size[0], size[1], "dashboard"
		endpoint := clientprofile.Endpoint{BaseURL: "http://127.0.0.1:30000/v1", Metadata: clientprofile.Metadata{ID: "fixture-qwen", ContextWindow: 262144, MaxTokens: 16384}}
		m.dashboard = dashboardui.NewEndpoint(context.Background(), endpoint, 4242, dashboardui.Dependencies{}).SetHealthy(true).WithSession(dashboardui.SessionInfo{Provider: "vast"})
		m.resizeChild()
		view := ansi.Strip(m.View())
		for _, want := range []string{"127.0.0.1:30000/v1", "fixture-qwen", "#4242"} {
			if !strings.Contains(view, want) {
				t.Errorf("%v missing core fact %s", size, want)
			}
		}
		if dir := os.Getenv("SOVKIT_PREVIEW_DIR"); dir != "" {
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("dashboard-embedded-%dx%d.txt", size[0], size[1])), []byte(view), 0600); err != nil {
				t.Fatal(err)
			}
		}
		m.cleanup()
	}
}

func TestExitFailureKeepsActionsVisible(t *testing.T) {
	for _, size := range [][2]int{{40, 14}, {80, 24}} {
		m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config.toml"), "root", "home", ApplicationDependencies{})
		m.width, m.height, m.screen = size[0], size[1], "dashboard"
		m.exitConfirm, m.exitInstance = true, 4242
		m.exitError = strings.Repeat("Provider could not stop this instance.\n", 20)
		view := ansi.Strip(m.View())
		for _, want := range []string{"Stop instance and quit", "Leave running and quit", "Destroy instance", "Cancel", "Provider"} {
			if !strings.Contains(view, want) {
				t.Errorf("%v lost %s", size, want)
			}
		}
		m.cleanup()
	}
}
