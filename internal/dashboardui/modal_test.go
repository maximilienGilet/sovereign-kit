package dashboardui

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestIntegrationConfirmationIsBoundedAndCentered(t *testing.T) {
	m := Model{width: 120, height: 40, page: "confirm"}
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	for i, line := range lines {
		if strings.Contains(line, "Review integration changes") {
			if i < 7 || strings.Index(line, "Review integration changes") < 20 {
				t.Fatalf("confirmation not centered at row %d: %q", i, line)
			}
			return
		}
	}
	t.Fatal("confirmation missing")
}
