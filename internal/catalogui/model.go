// Package catalogui provides the recipe-first terminal catalog.
package catalogui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
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

type entryItem struct{ Entry }

func (item entryItem) FilterValue() string { return item.Name + " " + item.Kind + " " + item.Status }
func (item entryItem) Title() string       { return item.Name + "  " + item.Status }
func (item entryItem) Description() string {
	return compact(item.Kind) + " · " + item.VRAM + " VRAM · " + item.Context
}

type Model struct {
	list   list.Model
	width  int
	height int
}

var (
	cyan       = lipgloss.Color("#00B8FF")
	muted      = lipgloss.Color("#7E98A8")
	ink        = lipgloss.Color("#E9F4F8")
	line       = lipgloss.Color("#385163")
	accent     = lipgloss.NewStyle().Foreground(cyan).Bold(true)
	label      = lipgloss.NewStyle().Foreground(muted)
	title      = lipgloss.NewStyle().Foreground(ink).Bold(true)
	panel      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(line).Padding(1, 2)
	metricName = lipgloss.NewStyle().Foreground(muted)
)

func New(entries []Entry) Model {
	items := make([]list.Item, 0, len(entries))
	for _, entry := range entries {
		items = append(items, entryItem{entry})
	}
	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(3)
	delegate.Styles.SelectedTitle = lipgloss.NewStyle().Foreground(cyan).BorderLeft(true).BorderStyle(lipgloss.ThickBorder()).BorderForeground(cyan).PaddingLeft(1).Bold(true)
	delegate.Styles.SelectedDesc = lipgloss.NewStyle().Foreground(ink).BorderLeft(true).BorderStyle(lipgloss.ThickBorder()).BorderForeground(cyan).PaddingLeft(1)
	delegate.Styles.NormalTitle = title
	delegate.Styles.NormalDesc = label
	catalog := list.New(items, delegate, 38, 24)
	catalog.Title = ""
	catalog.SetShowTitle(false)
	catalog.SetShowStatusBar(false)
	catalog.SetShowPagination(false)
	catalog.SetShowHelp(false)
	catalog.SetShowFilter(false)
	catalog.Styles.FilterPrompt = accent
	catalog.Styles.FilterCursor = accent
	return Model{list: catalog, width: 150, height: 42}
}

func (model Model) Init() tea.Cmd { return nil }

func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := message.(tea.WindowSizeMsg); ok {
		model.width, model.height = size.Width, size.Height
		model.resizeList()
	}
	var command tea.Cmd
	model.list, command = model.list.Update(message)
	return model, command
}

func (model Model) View() string {
	selected, ok := model.list.SelectedItem().(entryItem)
	if !ok {
		return title.Render("SOVEREIGN KIT") + "  " + label.Render("No launchable recipes") + "\n"
	}
	header := accent.Render("SOVEREIGN KIT") + label.Render("  /  CATALOG") +
		"\n" + label.Render("↑↓ browse  ·  / filter  ·  Enter inspect  ·  Space hardware  ·  L launch")
	leftWidth := clamp(model.width*32/100, 36, 48)
	rightWidth := max(52, model.width-leftWidth-3)
	body := lipgloss.JoinHorizontal(lipgloss.Top, model.recipeList(leftWidth), " ", detail(selected.Entry, rightWidth, model.height))
	return header + "\n\n" + body + "\n"
}

func (model *Model) resizeList() {
	leftWidth := clamp(model.width*32/100, 36, 48)
	model.list.SetSize(leftWidth-6, max(1, model.height-14))
}

func (model Model) recipeList(width int) string {
	search := model.list.FilterInput.View()
	content := accent.Render("RECIPES") + "\n" + search + "\n" + label.Render("Launchable routes and discovery flows") + "\n\n" + model.list.View()
	return panel.Width(width - 6).Render(content)
}

func detail(entry Entry, width, terminalHeight int) string {
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
	lines := []string{
		title.Render(entry.Name) + "  " + accent.Render(entry.Status),
		label.Render(entry.Kind),
		"",
		accent.Render("HARDWARE FIT"),
		metrics,
		"",
		performance,
	}
	if terminalHeight >= 28 {
		lines = append(lines, "", label.Render("SOURCE"), entry.Detail.Source, "", label.Render("NEXT STEP"), entry.Detail.Notes)
	}
	innerHeight := max(8, terminalHeight-8)
	return panel.Width(width - 6).Height(innerHeight).Render(strings.Join(lines, "\n"))
}

func metric(name, value string) string {
	return fmt.Sprintf("%-13s %s", metricName.Render(name), value)
}
func compact(value string) string           { return strings.ReplaceAll(value, "-generation", " gen") }
func clamp(value, minimum, maximum int) int { return min(max(value, minimum), maximum) }
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
