package catalogui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type modelMode int

const (
	browseMode modelMode = iota
	pickerMode
)

type pickerKeyMap struct {
	Navigate key.Binding
	Choose   key.Binding
	Evidence key.Binding
	Cancel   key.Binding
	Quit     key.Binding
}

func newPickerKeyMap() pickerKeyMap {
	return pickerKeyMap{
		Navigate: key.NewBinding(key.WithKeys("up", "down", "j", "k"), key.WithHelp("↑/↓", "recipe")),
		Choose:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "choose")),
		Evidence: key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "evidence")),
		Cancel:   key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc", "cancel")),
		Quit:     key.NewBinding(key.WithKeys("ctrl+c")),
	}
}

func (keys pickerKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.Navigate, keys.Choose, keys.Evidence, keys.Cancel}
}

func (keys pickerKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{keys.Navigate, keys.Choose}, {keys.Evidence, keys.Cancel}}
}

type evidenceKeyMap struct {
	Scroll key.Binding
	Close  key.Binding
}

func newEvidenceKeyMap() evidenceKeyMap {
	return evidenceKeyMap{
		Scroll: key.NewBinding(key.WithKeys("up", "down", "j", "k", "pgup", "pgdown"), key.WithHelp("↑/↓", "scroll")),
		Close:  key.NewBinding(key.WithKeys("i", "esc", "q"), key.WithHelp("i/esc", "back")),
	}
}

func (keys evidenceKeyMap) ShortHelp() []key.Binding { return []key.Binding{keys.Scroll, keys.Close} }
func (keys evidenceKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{keys.Scroll, keys.Close}}
}

func NewPicker(entries []Entry) Model {
	model := newModel(entries, pickerMode)
	model.list.SetFilteringEnabled(false)
	model.list.SetShowFilter(false)
	model.applyPickerAccent()
	model.resizePicker()
	model.initialAnimation = model.animateContext()
	return model
}

func (model Model) SelectedValue() (string, bool) {
	return model.selected, model.selected != "" && !model.cancelled
}

func (model Model) Cancelled() bool { return model.cancelled }

func (model Model) selectedEntry() (Entry, bool) {
	selected, ok := model.list.SelectedItem().(entryItem)
	if !ok {
		return Entry{}, false
	}
	return selected.Entry, true
}

func (model Model) updatePicker(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case progress.FrameMsg:
		updated, command := model.progress.Update(message)
		model.progress = updated.(progress.Model)
		return model, command
	case tea.WindowSizeMsg:
		model.width = max(message.Width, 1)
		model.height = max(message.Height, 1)
		model.resizePicker()
		if model.showDetail {
			model.setEvidenceContent()
		}
		return model, nil
	case tea.KeyMsg:
		if key.Matches(message, model.keys.Quit) {
			model.cancelled = true
			return model, model.pickerCompletion()
		}
		if model.showDetail {
			evidenceKeys := newEvidenceKeyMap()
			if key.Matches(message, evidenceKeys.Close) {
				model.showDetail = false
				return model, nil
			}
			var command tea.Cmd
			model.evidence, command = model.evidence.Update(message)
			return model, command
		}
		if key.Matches(message, model.keys.Cancel) {
			model.cancelled = true
			return model, model.pickerCompletion()
		}
		if key.Matches(message, model.keys.Evidence) {
			if _, ok := model.selectedEntry(); ok {
				model.showDetail = true
				model.setEvidenceContent()
			}
			return model, nil
		}
		if key.Matches(message, model.keys.Choose) {
			if selected, ok := model.selectedEntry(); ok && selected.Value != "" {
				model.selected = selected.Value
				return model, model.pickerCompletion()
			}
			return model, nil
		}
	}
	previousIndex := model.list.Index()
	var command tea.Cmd
	model.list, command = model.list.Update(message)
	if model.list.Index() != previousIndex {
		model.applyPickerAccent()
		animation := model.animateContext()
		return model, tea.Batch(command, animation)
	}
	return model, command
}

func (model *Model) applyPickerAccent() {
	entry, ok := model.selectedEntry()
	if !ok {
		return
	}
	color := recipeAccent(entry)
	model.progress.FullColor = string(color)
	model.list.SetDelegate(entryDelegate(pickerMode, color))
	model.help.Styles.ShortKey = cockpitAccent(entry)
}

func (model *Model) animateContext() tea.Cmd {
	ratio := 0.0
	if entry, ok := model.selectedEntry(); ok && entry.NativeContextWindow > 0 {
		ratio = float64(entry.ContextWindow) / float64(entry.NativeContextWindow)
	}
	return model.progress.SetPercent(ratio)
}

func (model Model) pickerSidebarWidth() int {
	return clamp(model.width*28/100, 24, 64)
}

// Lip Gloss Width/Height include padding, but not borders. Children receive
// the inner area, while panels reserve only the border outside their size.
func pickerPanel(width, height int) lipgloss.Style {
	return panel.Width(max(1, width-panel.GetHorizontalBorderSize())).
		Height(max(1, height-panel.GetVerticalBorderSize()))
}

func (model *Model) resizePicker() {
	model.help.Width = max(1, model.width-2)
	model.evidence.Width = max(1, model.width-panel.GetHorizontalFrameSize())
	model.evidence.Height = max(1, model.height-6)
	if model.width < 72 || model.height < 24 {
		model.list.SetSize(max(1, model.width-4), max(1, model.height-9))
	} else {
		leftWidth := model.pickerSidebarWidth()
		model.list.SetSize(max(1, leftWidth-panel.GetHorizontalFrameSize()), max(1, model.height-2-panel.GetVerticalFrameSize()-3))
	}
	// Reserve the default delegate's two-cell title inset before truncating.
	items := make([]list.Item, len(model.list.Items()))
	for index, item := range model.list.Items() {
		entry := item.(entryItem)
		entry.titleWidth = max(1, model.list.Width()-2)
		items[index] = entry
	}
	selected := model.list.Index()
	model.list.SetItems(items) // Filtering is disabled in the picker.
	model.list.Select(selected)
}

func (model *Model) setEvidenceContent() {
	entry, ok := model.selectedEntry()
	if !ok {
		model.evidence.SetContent("No recipe selected")
		return
	}
	width := model.evidence.Width
	if entry.Custom {
		model.evidence.SetContent(strings.Join([]string{
			title.Render("CHOOSE A MODEL"),
			wrappedFact("NAME", entry.Name, width),
			wrappedFact("NEXT STEPS", customWorkflow, width),
			wrappedFact("LIMITS", "Inspection does not guarantee performance or compatibility for every Hugging Face model. Existing compatibility classifications and refusals still apply.", width),
		}, "\n\n"))
		model.evidence.GotoTop()
		return
	}
	lines := []string{
		title.Render("EVIDENCE & TECHNICAL DETAILS"),
		"",
		wrappedFact("NAME", entry.Name, width),
		wrappedFact("PURPOSE", unknown(entry.Summary), width),
		wrappedFact("STATUS", entry.Status, width),
		wrappedFact("EVIDENCE", unknown(entry.Evidence), width),
		"",
		wrappedFact("MODEL", unknown(entry.ModelRepository), width),
		wrappedFact("REVISION", unknown(entry.ModelRevision), width),
		wrappedFact("RUNTIME", unknown(entry.Runtime), width),
		"",
		wrappedFact("CONTEXT", countOrUnknown(entry.ContextWindow, "tokens"), width),
		wrappedFact("NATIVE", countOrUnknown(entry.NativeContextWindow, "tokens"), width),
		wrappedFact("SOURCE", "SOURCE UNKNOWN; native limit is recipe-declared, not verified against the model config", width),
		wrappedFact("OUTPUT", countOrUnknown(entry.MaxOutputTokens, "tokens"), width),
		wrappedFact("CONCURRENT", requestCount(entry.MaxRunningRequests), width),
		wrappedFact("GPU", gpuContract(entry), width),
		wrappedFact("VRAM", countOrUnknown(entry.MinimumVRAMGB, "GB minimum / GPU"), width),
		wrappedFact("DISK", countOrUnknown(entry.MinimumDiskGB, "GB minimum / instance"), width),
		"",
		wrappedFact("THROUGHPUT", "NOT MEASURED for this exact profile", width),
		"",
		accent.Render("CHOOSE THIS RECIPE IF"),
	}
	for _, item := range entry.UseWhen {
		lines = append(lines, wrappedFact("CONDITION", item, width))
	}
	if len(entry.UseWhen) == 0 {
		lines = append(lines, wrappedFact("NEXT STEP", "Inspect the model before creating a custom recipe", width))
	}
	model.evidence.SetContent(strings.Join(lines, "\n"))
	model.evidence.GotoTop()
}

func (model Model) pickerView() string {
	if model.showDetail {
		return model.evidenceView()
	}
	if model.width < 72 || model.height < 24 {
		return model.minimalPickerView()
	}
	if model.width >= 134 && model.height >= 30 {
		return model.widePickerView()
	}
	return model.compactPickerView()
}

func (model Model) minimalPickerView() string {
	entry, ok := model.selectedEntry()
	if !ok {
		return fitView(title.Render("SOVEREIGN KIT")+"  "+label.Render("No launchable recipes"), model.width, model.height)
	}
	if entry.Custom {
		return fitView(customWelcome(entry, model.width, model.height-1)+"\n"+model.help.View(model.keys), model.width, model.height)
	}
	lines := []string{
		cockpitTitle(entry, model.width),
		metric("CONTEXT", countOrUnknown(entry.ContextWindow, "tokens")),
		metric("OUT / REQS", formatCatalogCount(entry.MaxOutputTokens)+" / "+requestCount(entry.MaxRunningRequests)),
	}
	if entry.Runtime == "llama-cpp" {
		lines = append(lines, metric("KV", entry.KVCacheTypeK+"/"+entry.KVCacheTypeV))
	}
	lines = append(lines,
		metric("GPU "+strings.ToUpper(strings.TrimSuffix(gpuPolicy(entry), " required")), gpuName(entry)),
		metric("VRAM / GPU", countOrUnknown(entry.MinimumVRAMGB, "GB min")),
		metric("DISK/INSTANCE", countOrUnknown(entry.MinimumDiskGB, "GB min")),
		metric("THROUGHPUT", "NOT MEASURED"),
		cockpitAccent(entry).Render("[ "+cockpitLine(actionLabel(entry), max(1, model.width-4))+" ]"),
		model.help.View(model.keys),
	)
	if model.height > len(lines)+1 {
		lines = append([]string{accent.Render("SOVEREIGN KIT") + label.Render("  /  CHOOSE A RECIPE")}, lines...)
	}
	return fitView(strings.Join(lines, "\n"), model.width, model.height)
}

func (model Model) compactPickerView() string { return model.pickerColumns(false) }
func (model Model) widePickerView() string    { return model.pickerColumns(true) }

func (model Model) pickerColumns(wide bool) string {
	entry, ok := model.selectedEntry()
	if !ok {
		return fitView(title.Render("SOVEREIGN KIT")+"  "+label.Render("No launchable recipes"), model.width, model.height)
	}
	leftWidth := model.pickerSidebarWidth()
	rightWidth := model.width - leftWidth - 1
	bodyHeight := model.height - 2
	left := model.pickerList(leftWidth, bodyHeight)
	right := pickerPanel(rightWidth, bodyHeight).Render(model.recipeDashboard(entry, max(1, rightWidth-panel.GetHorizontalFrameSize()), wide))
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
	view := strings.Join([]string{
		accent.Render("SOVEREIGN KIT") + label.Render("  /  SETUP  /  CHOOSE A RECIPE"),
		body,
		model.help.View(model.keys),
	}, "\n")
	return fitView(view, model.width, model.height)
}

func (model Model) pickerList(width, height int) string {
	content := accent.Render("RECIPES") + "\n" + label.Render("Choose one route") + "\n\n" + model.list.View()
	return pickerPanel(width, height).Render(content)
}

func (model Model) contextProgress(width int) string {
	bar := model.progress
	bar.Width = max(1, width)
	return bar.View()
}

func (model Model) evidenceView() string {
	keys := newEvidenceKeyMap()
	body := pickerPanel(model.width, model.height-2).Render(model.evidence.View())
	view := strings.Join([]string{accent.Render("SOVEREIGN KIT") + label.Render("  /  RECIPE EVIDENCE"), body, model.help.View(keys)}, "\n")
	return fitView(view, model.width, model.height)
}

func wrappedFact(name, value string, width int) string {
	prefix := fmt.Sprintf("%-11s", name)
	if width < lipgloss.Width(prefix)+8 {
		return ansi.Wrap(name+"\n"+value, max(1, width), "/")
	}
	valueWidth := max(1, width-lipgloss.Width(prefix))
	wrapped := ansi.Hardwrap(ansi.Wordwrap(value, valueWidth, "/"), valueWidth, false)
	parts := strings.Split(wrapped, "\n")
	lines := make([]string, 0, len(parts))
	for index, part := range parts {
		if index == 0 {
			lines = append(lines, metricName.Render(prefix)+part)
			continue
		}
		lines = append(lines, strings.Repeat(" ", lipgloss.Width(prefix))+part)
	}
	return strings.Join(lines, "\n")
}

func gpuContract(entry Entry) string {
	if entry.GPUCount < 1 || strings.TrimSpace(entry.GPUModel) == "" {
		return "unknown"
	}
	return gpuName(entry) + " · " + gpuPolicy(entry)
}

func requestCount(value int) string {
	if value < 1 {
		return "unknown"
	}
	if value == 1 {
		return "1 request"
	}
	return fmt.Sprintf("%d requests", value)
}

func actionLabel(entry Entry) string {
	if entry.Custom {
		return "INSPECT HUGGING FACE"
	}
	return "USE " + strings.ToUpper(strings.Join(strings.Fields(entry.Name), " "))
}

func formatPercent(value float64) string {
	return fmt.Sprintf("%.1f%%", value*100)
}

func unknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "UNKNOWN"
	}
	return value
}

func fitLine(value string, width int) string {
	return ansi.Truncate(value, max(1, width), "…")
}

func fitView(view string, width, height int) string {
	lines := strings.Split(view, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for index, line := range lines {
		lines[index] = ansi.Truncate(line, width, "")
	}
	return strings.Join(lines, "\n")
}
