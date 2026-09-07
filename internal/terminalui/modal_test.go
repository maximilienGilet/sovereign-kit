package terminalui

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestModalPreservesBackgroundAndFitsWideGlyphs(t *testing.T) {
	background := strings.Repeat(strings.Repeat("界", 60)+"\n", 36)
	result := Modal(background, "Cancel\nDestroy…", 120, 36, false)
	lines := strings.Split(result, "\n")
	if len(lines) != 36 || !strings.Contains(lines[0], "界") {
		t.Fatal("lost background")
	}
	for _, line := range lines {
		if ansi.StringWidth(line) != 120 {
			t.Fatalf("width %d", ansi.StringWidth(line))
		}
	}
	if !strings.Contains(result, "Cancel") || !strings.Contains(result, "Destroy…") {
		t.Fatal("choices missing")
	}
}

func TestModalSmallScreenKeepsAllActions(t *testing.T) {
	content := "Stop\nKeep running\nDestroy…\nCancel"
	if Modal("background", content, 30, 6, false) != content {
		t.Fatal("small layout hid actions")
	}
}

func TestModalDoesNotSplitWideGlyphAtLeftEdge(t *testing.T) {
	background := strings.Repeat(strings.Repeat("界", 60)+"\n", 36)
	for _, line := range strings.Split(Modal(background, "Cancel", 119, 36, false), "\n") {
		if ansi.StringWidth(line) != 119 {
			t.Fatalf("wide glyph shifted modal: width %d", ansi.StringWidth(line))
		}
	}
}
