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
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/muesli/termenv"
)

// Opt-in captures use local fixtures only, never a live provider or route.
func TestPolishPreviewFrames(t *testing.T) {
	dir := os.Getenv("SOVKIT_POLISH_PREVIEW_DIR")
	if dir == "" {
		t.Skip("opt-in renderer captures")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	for _, kind := range []string{"orbit", "intake", "link", "inspection", "activation"} {
		frame := stageMotif(kind, 2300*time.Millisecond, true)
		if err := os.WriteFile(filepath.Join(dir, kind+".ansi"), []byte(frame), 0600); err != nil {
			t.Fatal(err)
		}
	}
	gridCell := func(a time.Duration, r float64) string {
		return fmt.Sprintf("t=%-3.1fs r=%.2f\n%s", a.Seconds(), r, provisioningForge(forgeMeasured, r, a, -1, true, ""))
	}
	joinGrid := func(cells []string, cols int) string {
		lines := make([][]string, len(cells))
		height := 0
		for i, c := range cells {
			lines[i] = strings.Split(c, "\n")
			height = max(height, len(lines[i]))
		}
		var out strings.Builder
		for start := 0; start < len(lines); start += cols {
			for r := 0; r < height; r++ {
				if r > 0 || start > 0 {
					out.WriteByte('\n')
				}
				if start > 0 && r == 0 {
					out.WriteByte('\n')
				}
				for c := start; c < min(start+cols, len(lines)); c++ {
					if c > start {
						out.WriteString("    ")
					}
					if r < len(lines[c]) {
						out.WriteString(lines[c][r])
					}
				}
			}
		}
		return out.String()
	}
	var angleCells []string
	for _, a := range []time.Duration{0, 3 * time.Second, 6 * time.Second, 9 * time.Second, 12 * time.Second, 15 * time.Second} {
		angleCells = append(angleCells, gridCell(a, .5))
	}
	var ratioCells []string
	for _, a := range []time.Duration{0, 6 * time.Second} {
		for _, r := range []float64{.1, .35, .65, .9} {
			ratioCells = append(ratioCells, gridCell(a, r))
		}
	}
	for name, grid := range map[string]string{
		"angles": joinGrid(angleCells, 3),
		"ratios": joinGrid(ratioCells, 4),
	} {
		if err := os.WriteFile(filepath.Join(dir, "foundry-"+name+".ansi"), []byte(grid), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	defer m.cleanup()
	m.width, m.height, m.screen = 120, 40, "working"
	m.applyProgress(setup.Progress{Stage: setup.ProgressLaunching, InstanceID: 4242})
	m.applyServerLogs(setup.ServerLogs{Text: "SOVKIT_DOWNLOAD {\"current\": 60, \"total\": 100}", CheckedAt: time.Now()})
	for _, page := range []string{"download", "exit", "destroy"} {
		if page != "download" {
			m.exitConfirm, m.exitInstance = true, 4242
		}
		m.exitDestroy = page == "destroy"
		if err := os.WriteFile(filepath.Join(dir, page+".ansi"), []byte(m.View()), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
