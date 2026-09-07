package catalogui

import (
	"fmt"
	"hash/fnv"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Identity colors are deliberately independent of status and performance.
func recipeAccent(entry Entry) lipgloss.Color {
	palette := [...]lipgloss.Color{"#00B8FF", "#65DDB8", "#B4A1FF", "#F2C879"}
	identity := entry.Value
	if entry.Custom {
		identity = "custom"
	}
	if identity == "" {
		identity = entry.Name
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(identity))
	return palette[hash.Sum32()%uint32(len(palette))]
}

func cockpitAccent(entry Entry) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(recipeAccent(entry)).Bold(true)
}

func cockpitLine(value string, width int) string {
	return fitLine(strings.Join(strings.Fields(value), " "), width)
}

func cockpitTitle(entry Entry, width int) string {
	badge := cockpitAccent(entry).Render("[ " + strings.Join(strings.Fields(unknown(entry.Status)), " ") + " ]")
	nameWidth := width - lipgloss.Width(badge) - 2
	if nameWidth < 1 {
		return fitLine(badge, width)
	}
	return title.Render(cockpitLine(entry.Name, nameWidth)) + "  " + badge
}

func cockpitHeader(entry Entry, width int) string {
	return cockpitTitle(entry, width) + "\n" + label.Render(cockpitLine(entry.Summary, width))
}

func (model Model) recipeDashboard(entry Entry, width int, wide bool) string {
	if entry.Custom {
		return customWelcome(entry, width, model.height-2-panel.GetVerticalFrameSize())
	}
	readingWidth := min(width, 160)
	if !wide || readingWidth < 88 {
		return model.compactDashboard(entry, width)
	}
	leftWidth := (readingWidth - 3) / 2
	rightWidth := readingWidth - leftWidth - 3
	capacity := model.capacityZone(entry, leftWidth)
	hardware := hardwareZone(entry, rightWidth)
	zones := lipgloss.JoinHorizontal(lipgloss.Top, capacity, "   ", hardware)
	footer := label.Render("THROUGHPUT  NOT MEASURED") + "\n" +
		cockpitAccent(entry).Render("[ "+actionLabel(entry)+" ]") + label.Render("    i evidence & all use cases")
	content := strings.Join([]string{cockpitHeader(entry, readingWidth), "", zones, "", footer}, "\n")
	bodyHeight := model.height - 2 - panel.GetVerticalFrameSize()
	if lipgloss.Height(content) > bodyHeight || lipgloss.Width(content) > readingWidth {
		return model.compactDashboard(entry, width)
	}
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, content)
}

func (model Model) capacityZone(entry Entry, width int) string {
	accent := cockpitAccent(entry)
	lines := []string{accent.Render("CONTEXT · CONFIGURED / NATIVE")}
	if entry.NativeContextWindow > 0 && entry.ContextWindow > 0 {
		ratio := float64(entry.ContextWindow) / float64(entry.NativeContextWindow)
		lines = append(lines,
			fmt.Sprintf("%s / %s  %s", formatCatalogCount(entry.ContextWindow), formatCatalogCount(entry.NativeContextWindow), formatPercent(ratio)),
			accent.Render(contextMarker(ratio, width)), model.contextProgress(width),
			label.Render(contextTicks(width)),
			label.Render("0"+strings.Repeat(" ", max(1, width-1-len(formatCatalogCount(entry.NativeContextWindow)+" tokens")))+formatCatalogCount(entry.NativeContextWindow)+" tokens"),
			label.Render("SOURCE UNKNOWN · recipe-declared"),
		)
	} else {
		lines = append(lines, countOrUnknown(entry.ContextWindow, "tokens"), label.Render("NATIVE CONTEXT UNKNOWN"))
	}
	lines = append(lines, metric("OUTPUT", countOrUnknown(entry.MaxOutputTokens, "tokens maximum")))
	if entry.Runtime == "llama-cpp" {
		lines = append(lines, metric("KV CACHE", kvCacheLabel(entry)))
	}
	lines = append(lines, accent.Render("CONFIGURED CAPACITY"), concurrencySlots(entry.MaxRunningRequests, width, recipeAccent(entry)))
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}

func contextMarker(ratio float64, width int) string {
	width = max(1, width)
	ratio = math.Max(0, math.Min(1, ratio))
	position := int(math.Round(ratio * float64(width-1)))
	return strings.Repeat(" ", position) + "▼" + strings.Repeat(" ", width-position-1)
}

func contextTicks(width int) string {
	width = max(1, width)
	ticks := []rune(strings.Repeat("─", width))
	for tick := 0; tick <= 4; tick++ {
		position := int(math.Round(float64(tick) * float64(width-1) / 4))
		ticks[position] = '┬'
	}
	return string(ticks)
}

func concurrencySlots(count, width int, color lipgloss.Color) string {
	if count < 1 {
		return label.Render("UNKNOWN · inspect model first")
	}
	visible := min(count, min(8, (width+1)/5))
	for visible > 0 && visible < count && visible*5+len(fmt.Sprintf("+%s more", formatCatalogCount(count-visible))) > width {
		visible--
	}
	parts := make([]string, 0, visible+1)
	for slot := 1; slot <= visible; slot++ {
		parts = append(parts, lipgloss.NewStyle().Border(lipgloss.NormalBorder()).
			BorderForeground(color).Render(fmt.Sprintf("%02d", slot)))
	}
	if visible < count {
		parts = append(parts, label.PaddingTop(1).Render(fmt.Sprintf("+%s more", formatCatalogCount(count-visible))))
	}
	var row string
	for index, part := range parts {
		if index == 0 {
			row = part
		} else {
			row = lipgloss.JoinHorizontal(lipgloss.Top, row, " ", part)
		}
	}
	return row + "\n" + label.Render(cockpitLine(requestCount(count)+" max · not live usage", width))
}

func hardwareZone(entry Entry, width int) string {
	accent := cockpitAccent(entry)
	innerWidth := width - 4
	name := gpuName(entry)
	policy := gpuPolicy(entry)
	if entry.Custom {
		name, policy = "MODEL NOT SELECTED", "inspection required"
	}
	chip := lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(recipeAccent(entry)).
		Padding(0, 1).Width(width - 2).Render(strings.Join([]string{
		accent.Render(cockpitLine(name, innerWidth)),
		label.Render(policy),
		cockpitLine(countOrUnknown(entry.MinimumVRAMGB, "GB minimum / GPU"), innerWidth),
	}, "\n"))
	lines := []string{accent.Render("ACCELERATOR CONTRACT"), chip,
		cockpitLine(countOrUnknown(entry.MinimumDiskGB, "GB minimum / instance"), width), "",
		accent.Render("CHOOSE THIS RECIPE IF")}
	for _, item := range entry.UseWhen[:min(3, len(entry.UseWhen))] {
		lines = append(lines, cockpitLine("◆ "+item, width))
	}
	if len(entry.UseWhen) == 0 {
		lines = append(lines, cockpitLine("Inspect the model before creating a custom recipe", width))
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}

func gpuName(entry Entry) string {
	if entry.GPUCount < 1 || strings.TrimSpace(entry.GPUModel) == "" {
		return "UNKNOWN"
	}
	return fmt.Sprintf("%d× %s", entry.GPUCount, strings.Join(strings.Fields(entry.GPUModel), " "))
}

func gpuPolicy(entry Entry) string {
	if entry.GPUCount < 1 || strings.TrimSpace(entry.GPUModel) == "" {
		return "not specified"
	}
	if entry.StrictGPU {
		return "exact required"
	}
	return "preferred"
}

func (model Model) compactDashboard(entry Entry, width int) string {
	accent := cockpitAccent(entry)
	lines := []string{cockpitHeader(entry, width), accent.Render("CONFIGURED CONTEXT / MODEL NATIVE")}
	if entry.NativeContextWindow > 0 && entry.ContextWindow > 0 {
		ratio := float64(entry.ContextWindow) / float64(entry.NativeContextWindow)
		lines = append(lines, fmt.Sprintf("%s / %s  %s", formatCatalogCount(entry.ContextWindow), formatCatalogCount(entry.NativeContextWindow), formatPercent(ratio)),
			model.contextProgress(min(width, 160)), label.Render("SOURCE UNKNOWN · recipe-declared"))
	} else {
		lines = append(lines, countOrUnknown(entry.ContextWindow, "tokens"), label.Render("NATIVE CONTEXT UNKNOWN"))
	}
	lines = append(lines,
		metric("OUTPUT", countOrUnknown(entry.MaxOutputTokens, "tokens")),
		metric("CONCURRENT", requestCount(entry.MaxRunningRequests)))
	if entry.Runtime == "llama-cpp" {
		lines = append(lines, metric("KV CACHE", kvCacheLabel(entry)))
	}
	lines = append(lines,
		metric("GPU", gpuName(entry)), metric("GPU CONTRACT", gpuPolicy(entry)),
		metric("VRAM / GPU", countOrUnknown(entry.MinimumVRAMGB, "GB minimum")),
		metric("DISK/INSTANCE", countOrUnknown(entry.MinimumDiskGB, "GB minimum")),
		accent.Render("CHOOSE THIS RECIPE IF"))
	if len(entry.UseWhen) > 0 {
		lines = append(lines, cockpitLine("◆ "+entry.UseWhen[0], width))
	} else {
		lines = append(lines, "Inspect the model before creating a recipe")
	}
	lines = append(lines, label.Render("i evidence & all use cases"), metric("THROUGHPUT", "NOT MEASURED"), accent.Render("[ "+actionLabel(entry)+" ]"))
	// Every variable field is a single measured line. Full text lives in i.
	content := strings.Join(lines, "\n")
	rows := strings.Split(content, "\n")
	for index, row := range rows {
		rows[index] = fitLine(row, width)
	}
	return strings.Join(rows, "\n")
}

func kvCacheLabel(entry Entry) string {
	k, v := strings.TrimSpace(entry.KVCacheTypeK), strings.TrimSpace(entry.KVCacheTypeV)
	if k == "" || v == "" {
		return "unknown"
	}
	return k + " K / " + v + " V"
}
