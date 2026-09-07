package dashboardui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestPolishPreviewFrames(t *testing.T) {
	dir := os.Getenv("SOVKIT_POLISH_PREVIEW_DIR")
	if dir == "" {
		t.Skip("opt-in fixture renderer capture")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	m := focusInspectorFixture(180)
	m.height = 55
	m = m.WithServerLogs("Fixture: model ready", time.Now())
	if err := os.WriteFile(filepath.Join(dir, "dashboard.ansi"), []byte(m.View()), 0600); err != nil {
		t.Fatal(err)
	}
}
