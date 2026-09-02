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
	width   int
	height  int
}

var (
	cyan       = lipgloss.Color("#00B8FF")
	muted      = lipgloss.Color("#7E98A8")
	ink        = lipgloss.Color("#E9F4F8")
	line       = lipgloss.Color("#385163")
	accent     = lipgloss.NewStyle().Foreground(cyan).Bold(true)
	label      = lipgloss.NewStyle().Foreground(muted)
	title      = lipgloss.NewStyle().Foreground(ink).Bold(true)
	selected   = lipgloss.NewStyle().Foreground(ink).Background(lipgloss.Color("#17394E"))
	panel      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(line).Padding(1, 2)
	metricName = lipgloss.NewStyle().Foreground(muted)
)

func New(entries []Entry) Model { return Model{entries: entries, width: 150, height: 42} }

func (model Model) Init() tea.Cmd { return nil }

func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch value := message.(type) {
	case tea.WindowSizeMsg:
		model.width, model.height = value.Width, value.Height
	case tea.KeyMsg:
		if len(model.entries) == 0 {
			return model, nil
		}
		switch value.Type {
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
		return title.Render("SOVEREIGN KIT") + "  " + label.Render("No launchable recipes") + "\n"
	}
	entry := model.entries[model.cursor]
	header := accent.Render("SOVEREIGN KIT") + label.Render("  /  CATALOG") +
		"\n" + label.Render("Choose a private model route.  ↑↓ browse  ·  Enter inspect  ·  Space hardware  ·  L launch")

	leftWidth := clamp(model.width*32/100, 36, 48)
	rightWidth := max(52, model.width-leftWidth-3)
	body := lipgloss.JoinHorizontal(lipgloss.Top, recipeList(model.entries, model.cursor, leftWidth), " ", detail(entry, rightWidth))
	return header + "\n\n" + body + "\n"
}

func recipeList(entries []Entry, cursor, width int) string {
	lines := []string{accent.Render("RECIPES"), label.Render("Launchable routes and discovery flows"), ""}
	for index, entry := range entries {
		marker := "  "
		if index == cursor {
			marker = "› "
		}
		name := marker + entry.Name
		subtitle := "  " + compact(entry.Kind) + " · " + entry.VRAM + " VRAM · " + entry.Context
		statusLine := "  " + entry.Status
		if index == cursor {
			lines = append(lines, selected.Width(width-6).Render(name), selected.Width(width-6).Render(subtitle), selected.Width(width-6).Render(statusLine), "")
		} else {
			lines = append(lines, title.Render(name), label.Render(subtitle), label.Render(statusLine), "")
		}
	}
	return panel.Width(width - 6).Height(max(18, len(lines)+2)).Render(strings.Join(lines, "\n"))
}

func detail(entry Entry, width int) string {
	metrics := strings.Join([]string{
		metric("MIN. VRAM", entry.VRAM),
		metric("CONTEXT", entry.Context),
		metric("OFFER COST", entry.Cost),
	}, "\n")
	performance := strings.Join([]string{
		accent.Render("PERFORMANCE"),
		entry.Speed,
		label.Render(entry.Detail.Confidence),
	}, "\n")
	provenance := strings.Join([]string{
		label.Render("SOURCE"),
		entry.Detail.Source,
		"",
		label.Render("NEXT STEP"),
		entry.Detail.Notes,
	}, "\n")
	lines := []string{
		title.Render(entry.Name) + "  " + accent.Render(entry.Status),
		label.Render(entry.Kind),
		"",
		accent.Render("HARDWARE FIT"),
		metrics,
		"",
		performance,
		"",
		provenance,
	}
	return panel.Width(width - 6).Height(max(18, 26)).Render(strings.Join(lines, "\n"))
}

func metric(name, value string) string {
	return fmt.Sprintf("%-13s %s", metricName.Render(name), value)
}

func compact(value string) string {
	return strings.ReplaceAll(value, "-generation", " gen")
}

func clamp(value, minimum, maximum int) int {
	return min(max(value, minimum), maximum)
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
