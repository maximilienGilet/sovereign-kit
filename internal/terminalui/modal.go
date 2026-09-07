package terminalui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Modal preserves the underlying screen but never delegates keyboard focus to it.
// On small terminals omit decoration rather than hide a destructive choice.
func Modal(background, content string, screenWidth, screenHeight int, color bool) string {
	ink := func(value, tone string) string {
		if !color {
			return value
		}
		return lipgloss.NewStyle().Foreground(lipgloss.Color(tone)).Render(value)
	}
	height := max(1, screenHeight)
	width := min(68, screenWidth-4)
	if width < 40 || height < 18 {
		return content
	}
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Width(width - 2)
	if color {
		style = style.BorderForeground(lipgloss.Color("#60DBC0"))
	}
	panel := style.Render(ansi.Wrap(content, width-6, ""))
	rows := strings.Split(panel, "\n")
	if len(rows) > height {
		return content
	}
	left, top := (screenWidth-lipgloss.Width(panel))/2, (height-len(rows))/2
	bg := strings.Split(ansi.Strip(background), "\n")
	result := make([]string, height)
	for y := range result {
		line := ""
		if y < len(bg) {
			line = ansi.Truncate(bg[y], screenWidth, "")
		}
		line += strings.Repeat(" ", max(0, screenWidth-ansi.StringWidth(line)))
		if y >= top && y < top+len(rows) {
			right := left + lipgloss.Width(panel)
			prefix := ansi.Truncate(line, right, "")
			// Fill any half of a wide glyph covered by the panel with a space.
			suffix := line[len(prefix):]
			if ansi.StringWidth(prefix) < right {
				runes := []rune(suffix)
				if len(runes) > 0 {
					suffix = " " + string(runes[1:])
				}
			}
			before := ansi.Truncate(line, left, "")
			before += strings.Repeat(" ", left-ansi.StringWidth(before))
			result[y] = ink(before, "#294650") + rows[y-top] + ink(suffix, "#294650")
		} else {
			result[y] = ink(line, "#294650")
		}
	}
	return strings.Join(result, "\n")
}
