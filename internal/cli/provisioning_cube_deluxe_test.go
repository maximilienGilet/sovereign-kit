package cli

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestCubeDeluxeLightingHasContinuousMaterialShading(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	frame := provisioningCube(1234*time.Millisecond, time.Second, true, "")
	colors := map[string]bool{}
	for _, color := range regexp.MustCompile(`\x1b\[38;2;[0-9;]+m`).FindAllString(frame, -1) {
		colors[color] = true
	}
	if len(colors) < 16 {
		t.Fatalf("crystalline surfaces need continuous shading rather than flat on/off lighting: %d tones", len(colors))
	}
}

func TestCubeDeluxeFramesKeepLayoutAndTerminalStatesStable(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	base := ansi.Strip(provisioningCube(0, -1, true, ""))
	for i := 0; i < 80; i++ {
		elapsed := time.Duration(i) * 80 * time.Millisecond
		frame := provisioningCube(elapsed, elapsed, true, "")
		if ansi.Strip(frame) != base {
			t.Fatalf("geometry moved at frame %d", i)
		}
		lines := strings.Split(ansi.Strip(frame), "\n")
		if len(lines) != 19 {
			t.Fatalf("height: %d", len(lines))
		}
		for _, line := range lines {
			if ansi.StringWidth(line) != 41 {
				t.Fatal("cube changed width")
			}
		}
		if provisioningCube(elapsed, elapsed, false, "") != base {
			t.Fatal("NO_COLOR must remain static")
		}
		for _, marker := range []string{"!", "✓"} {
			if provisioningCube(elapsed, elapsed, true, marker) != provisioningCube(0, -1, true, marker) {
				t.Fatalf("terminal state %s still animates", marker)
			}
		}
	}
}
