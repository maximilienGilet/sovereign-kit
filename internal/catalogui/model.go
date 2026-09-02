// Package catalogui provides the recipe-first terminal catalog.
package catalogui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Entry struct {
	Name    string
	Kind    string
	VRAM    string
	Context string
	Speed   string
	Cost    string
	Status  string
	Detail  Detail
}

type Detail struct {
	Source     string
	Confidence string
	Notes      string
}

type Model struct {
	entries []Entry
	cursor  int
}

var (
	cyan     = lipgloss.Color("#00AEEF")
	muted    = lipgloss.Color("#93A1B5")
	ink      = lipgloss.Color("#EAF0F8")
	selected = lipgloss.NewStyle().Foreground(cyan).Background(lipgloss.Color("#33465E"))
	label    = lipgloss.NewStyle().Foreground(muted)
	title    = lipgloss.NewStyle().Foreground(ink).Bold(true)
	status   = lipgloss.NewStyle().Foreground(cyan).Bold(true)
)

func New(entries []Entry) Model { return Model{entries: entries} }

func (model Model) Init() tea.Cmd { return nil }

func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyMsg); ok && len(model.entries) > 0 {
		switch key.Type {
		case tea.KeyDown, tea.KeyCtrlN:
			model.cursor = min(model.cursor+1, len(model.entries)-1)
		case tea.KeyUp, tea.KeyCtrlP:
			model.cursor = max(model.cursor-1, 0)
		}
	}
	return model, nil
}

func (model Model) View() string {
	if len(model.entries) == 0 {
		return title.Render("CATALOG") + "  " + label.Render("No launchable recipes") + "\n"
	}
	selectedEntry := model.entries[model.cursor]
	header := title.Render("CATALOG") + label.Render("  ·  choose a private model route") +
		"\n" + label.Render("↑↓ navigate  ·  Enter inspect  ·  Space plan hardware  ·  L launch  ·  Esc close")

	rows := []string{label.Render("  RECIPE                         KIND                 VRAM      CONTEXT   SPEED                    COST       STATUS")}
	for index, entry := range model.entries {
		prefix := "  "
		if index == model.cursor {
			prefix = "› "
		}
		row := fmt.Sprintf("%s%-30s %-20s %-9s %-9s %-24s %-10s %s", prefix, entry.Name, entry.Kind, entry.VRAM, entry.Context, entry.Speed, entry.Cost, entry.Status)
		if index == model.cursor {
			rows = append(rows, selected.Render(row))
		} else {
			rows = append(rows, row)
		}
	}
	left := lipgloss.NewStyle().Width(122).Render(strings.Join(rows, "\n"))
	right := detail(selectedEntry)
	return header + "\n\n" + lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right) + "\n"
}

func detail(entry Entry) string {
	lines := []string{
		title.Render(entry.Name) + "  " + status.Render(entry.Status),
		label.Render(entry.Kind),
		"",
		title.Render("HARDWARE"),
		"VRAM floor  " + entry.VRAM,
		"Context     " + entry.Context,
		"Cost        " + entry.Cost,
		"",
		title.Render("PERFORMANCE"),
		entry.Speed,
		entry.Detail.Confidence,
		"",
		label.Render("SOURCE  " + entry.Detail.Source),
		"",
		label.Render("NOTES   " + entry.Detail.Notes),
	}
	return lipgloss.NewStyle().Width(48).BorderLeft(true).BorderForeground(lipgloss.Color("#52657C")).PaddingLeft(3).Render(strings.Join(lines, "\n"))
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
