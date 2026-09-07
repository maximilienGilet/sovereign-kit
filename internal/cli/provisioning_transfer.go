package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Milestones, not a time estimate. Only the active segment shimmers.
func (l provisioningLoader) stepBar(width int, color, frozen bool) string {
	n := len(l.completed) + len(l.future) + 1
	if n <= 1 || width < n*2-1 {
		return ""
	}
	segment := strings.Repeat("━", max(1, (width-(n-1))/n))
	parts := make([]string, n)
	for i := range parts {
		part := segment
		tone := "#294650"
		if i < len(l.completed) {
			tone = "#5ADABD"
		}
		if i == len(l.completed) {
			tone = "#59B9CA"
			if frozen {
				tone = "#EAB060"
			}
		}
		if color {
			if i == len(l.completed) && !frozen {
				part = shimmer(part, l.now.Sub(l.started))
			} else {
				part = lipgloss.NewStyle().Foreground(lipgloss.Color(tone)).Render(part)
			}
		} else if i > len(l.completed) {
			part = strings.Repeat("┄", len([]rune(segment)))
		}
		parts[i] = part
	}
	return strings.Join(parts, " ") + "\n" + ansi.Truncate(fmt.Sprintf("%d/%d steps completed", len(l.completed), n), width, "…")
}

func transferBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1f KiB", float64(n)/1024)
	}
	if n < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MiB", float64(n)/(1024*1024))
	}
	return fmt.Sprintf("%.2f GiB", float64(n)/(1024*1024*1024))
}

func (l provisioningLoader) transferView(width int, color, meter bool) string {
	label := "MODEL DOWNLOAD"
	detail := l.transferDetail()
	bar := ""
	if l.transferTotal > 0 && l.transferCurrent >= 0 && l.transferCurrent <= l.transferTotal {
		ratio := float64(l.transferCurrent) / float64(l.transferTotal)
		if meter {
			p := progress.New(progress.WithSolidFill("#5ADABD"), progress.WithoutPercentage())
			p.Width = width
			bar = p.ViewAs(ratio)
			if !color {
				bar = ansi.Strip(bar)
			}
		}
	}
	parts := []string{label}
	if bar != "" {
		parts = append(parts, bar)
	}
	parts = append(parts, ansi.Truncate(detail, width, "…"))
	return strings.Join(parts, "\n")
}

func (l provisioningLoader) transferDetail() string {
	if l.transferTotal > 0 && l.transferCurrent >= 0 && l.transferCurrent <= l.transferTotal {
		ratio := float64(l.transferCurrent) / float64(l.transferTotal)
		return fmt.Sprintf("%s / %s · %.1f%%", transferBytes(l.transferCurrent), transferBytes(l.transferTotal), ratio*100)
	}
	return transferBytes(l.transferCurrent) + " · total unknown"
}
