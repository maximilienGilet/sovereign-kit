// Package catalogui provides the recipe-first terminal catalog.
package catalogui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Entry struct {
	Value               string
	Name                string
	Kind                string
	Status              string
	Summary             string
	Evidence            string
	UseWhen             []string
	ModelRepository     string
	ModelRevision       string
	Runtime             string
	GPUModel            string
	GPUCount            int
	StrictGPU           bool
	ContextWindow       int
	NativeContextWindow int
	MaxOutputTokens     int
	MaxRunningRequests  int
	KVCacheTypeK        string
	KVCacheTypeV        string
	MinimumVRAMGB       int
	MinimumDiskGB       int
	Custom              bool
}

type entryItem struct {
	Entry
	picker     bool
	titleWidth int
}

func (item entryItem) FilterValue() string { return item.Name + " " + item.Kind + " " + item.Status }
func (item entryItem) Title() string {
	if item.picker && item.titleWidth > 0 {
		status := strings.Join(strings.Fields(unknown(item.Status)), " ")
		nameWidth := item.titleWidth - lipgloss.Width(status) - 2
		if nameWidth < lipgloss.Width(strings.Join(strings.Fields(item.Name), " ")) {
			// Keep identity first; the status moves to the description below.
			return cockpitLine(item.Name, item.titleWidth)
		}
		return cockpitLine(item.Name, nameWidth) + "  " + status
	}
	return item.Name + "  " + item.Status
}
func (item entryItem) Description() string {
	if item.picker {
		status := strings.Join(strings.Fields(unknown(item.Status)), " ")
		prefix := ""
		if item.titleWidth > 0 && lipgloss.Width(strings.Join(strings.Fields(item.Name), " "))+lipgloss.Width(status)+2 > item.titleWidth {
			prefix = status + "\n"
		}
		if summary := strings.TrimSpace(item.Summary); summary != "" {
			return prefix + summary
		}
		return prefix + compact(item.Kind)
	}
	return compact(item.Kind) + " · " + countOrUnknown(item.MinimumVRAMGB, "GB") + " VRAM · " + countOrUnknown(item.ContextWindow, "context")
}

type Model struct {
	list             list.Model
	progress         progress.Model
	help             help.Model
	evidence         viewport.Model
	mode             modelMode
	width            int
	height           int
	selected         string
	cancelled        bool
	embedded         bool
	showDetail       bool
	keys             pickerKeyMap
	initialAnimation tea.Cmd
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

func New(entries []Entry) Model { return newModel(entries, browseMode) }

func newModel(entries []Entry, mode modelMode) Model {
	items := make([]list.Item, 0, len(entries))
	for _, entry := range entries {
		items = append(items, entryItem{Entry: entry, picker: mode == pickerMode})
	}
	delegate := entryDelegate(mode, cyan)
	catalog := list.New(items, delegate, 38, 24)
	catalog.Title = ""
	catalog.SetShowTitle(false)
	catalog.SetShowStatusBar(false)
	catalog.SetShowPagination(false)
	catalog.SetShowHelp(false)
	catalog.SetShowFilter(false)
	catalog.Styles.FilterPrompt = accent
	catalog.Styles.FilterCursor = accent
	bar := progress.New(progress.WithSolidFill("#00B8FF"), progress.WithSpringOptions(30, 1))
	bar.Empty = '─'
	bar.Full = '█'
	bar.ShowPercentage = false
	helpModel := help.New()
	helpModel.Styles.ShortKey = accent
	helpModel.Styles.ShortDesc = label
	helpModel.Styles.ShortSeparator = label
	return Model{
		list:     catalog,
		progress: bar,
		help:     helpModel,
		evidence: viewport.New(1, 1),
		mode:     mode,
		width:    150,
		height:   42,
		keys:     newPickerKeyMap(),
	}
}

func entryDelegate(mode modelMode, color lipgloss.Color) list.DefaultDelegate {
	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(3)
	delegate.Styles.SelectedTitle = lipgloss.NewStyle().Foreground(color).BorderLeft(true).BorderStyle(lipgloss.ThickBorder()).BorderForeground(color).PaddingLeft(1).Bold(true)
	delegate.Styles.SelectedDesc = lipgloss.NewStyle().Foreground(ink).BorderLeft(true).BorderStyle(lipgloss.ThickBorder()).BorderForeground(color).PaddingLeft(1)
	delegate.Styles.NormalTitle = title
	delegate.Styles.NormalDesc = label
	if mode == pickerMode {
		// DefaultDelegate measures against the normal style's inset.
		delegate.Styles.NormalTitle = title.PaddingLeft(2)
		delegate.Styles.NormalDesc = label.PaddingLeft(2)
	}
	return delegate
}

func (model Model) Init() tea.Cmd { return model.initialAnimation }

func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if model.mode == pickerMode {
		return model.updatePicker(message)
	}
	if size, ok := message.(tea.WindowSizeMsg); ok {
		model.width, model.height = size.Width, size.Height
		model.resizeList()
	}
	var command tea.Cmd
	model.list, command = model.list.Update(message)
	return model, command
}

func (model Model) View() string {
	if model.mode == pickerMode {
		return model.pickerView()
	}
	selected, ok := model.list.SelectedItem().(entryItem)
	if !ok {
		return title.Render("SOVEREIGN KIT") + "  " + label.Render("No launchable recipes") + "\n"
	}
	header := accent.Render("SOVEREIGN KIT") + label.Render("  /  CATALOG")
	help := label.Render("↑↓ browse  ·  / filter  ·  q close")
	leftWidth := clamp(model.width*32/100, 36, 48)
	rightWidth := max(52, model.width-leftWidth-3)
	body := lipgloss.JoinHorizontal(lipgloss.Top, model.recipeList(leftWidth), " ", detail(selected.Entry, rightWidth, model.height))
	return header + "\n" + help + "\n\n" + body + "\n"
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
		metric("MIN. VRAM", countOrUnknown(entry.MinimumVRAMGB, "GB")),
		metric("CONTEXT", countOrUnknown(entry.ContextWindow, "context")),
		metric("OFFER COST", "search offers"),
	}, "\n")
	performance := strings.Join([]string{
		accent.Render("PERFORMANCE"),
		entry.Summary,
		label.Render(entry.Evidence),
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
		lines = append(lines, "", label.Render("SOURCE"), entry.ModelRepository, "", label.Render("NEXT STEP"), entry.GPUModel)
	}
	innerHeight := max(8, terminalHeight-8)
	return panel.Width(width - 6).Height(innerHeight).Render(strings.Join(lines, "\n"))
}

func metric(name, value string) string {
	return fmt.Sprintf("%-13s %s", metricName.Render(name), value)
}
func countOrUnknown(value int, suffix string) string {
	if value < 1 {
		return "unknown"
	}
	return formatCatalogCount(value) + " " + suffix
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
