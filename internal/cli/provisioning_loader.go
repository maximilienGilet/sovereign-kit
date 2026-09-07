package cli

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type provisioningTick struct {
	generation, epoch uint64
	at                time.Time
}
type provisioningLoader struct {
	stageID, serverPhase  string
	epoch                 uint64
	active                bool
	started, now, changed time.Time
	forgeStarted          time.Time
	label                 string
	completed             []string
	future                []string
	spinner               spinner.Model
	activity              []string
	providerStatus        string
	checkedAt             time.Time
	activityChanged       time.Time
	detailSnapshot        string
	providerMessage       string
	transferCurrent       int64
	transferTotal         int64
	transferActive        bool
	transferRatioHigh     float64
	transferCompleted     bool
	transferCompletedAt   time.Time
}

func (l *provisioningLoader) stage(label string, at time.Time) {
	if l.forgeStarted.IsZero() {
		l.forgeStarted = at
	}
	if label == l.label {
		return
	}
	l.label, l.started, l.changed, l.now = label, at, at, at
}

func (l provisioningLoader) forgeElapsed() time.Duration {
	if l.forgeStarted.IsZero() {
		return 0
	}
	return max(time.Duration(0), l.now.Sub(l.forgeStarted))
}

// One root-owned clock drives both geometry and the active label. Epochs retire
// outstanding timers when a prompt, confirmation, or error suspends the screen.
func (m *applicationModel) reconcileLoader() tea.Cmd {
	active := m.loaderAllowed()
	l := &m.loader
	if !active {
		if l.active {
			l.active = false
			l.epoch++
		}
		return nil
	}
	l.stage(m.status, time.Now())
	if m.screen != "working" {
		l.future = nil
	}
	if l.active {
		return nil
	}
	l.active = true
	l.epoch++
	l.spinner = spinner.New(spinner.WithSpinner(spinner.Dot))
	return m.loaderTick()
}
func (m *applicationModel) loaderAllowed() bool {
	if m.exitConfirm || m.quitting || m.afterStop != "" {
		return false
	}
	if m.screen == "prompt" {
		browser, ok := m.child.(*offerBrowserModel)
		return ok && browser.loading && !browser.closed
	}
	return m.screen == "working" || m.screen == "connecting" || m.screen == "destroying"
}
func (m *applicationModel) loaderTick() tea.Cmd {
	generation, epoch := m.generation, m.loader.epoch
	return tea.Tick(80*time.Millisecond, func(at time.Time) tea.Msg { return provisioningTick{generation, epoch, at} })
}
func (m *applicationModel) updateLoader(t provisioningTick) tea.Cmd {
	l := &m.loader
	if !l.active || t.generation != m.generation || t.epoch != l.epoch || !m.loaderAllowed() {
		return nil
	}
	l.now = t.at
	l.spinner, _ = l.spinner.Update(spinner.TickMsg{Time: t.at, ID: l.spinner.ID()})
	if browser, ok := m.child.(*offerBrowserModel); ok && browser.loading {
		browser.loadingFrame = l.spinner.View()
	}
	return m.loaderTick()
}

func (l provisioningLoader) view(width, height int, color bool, marker string) string {
	elapsed := l.now.Sub(l.started)
	if elapsed < 0 || l.started.IsZero() {
		elapsed = 0
	}
	label := l.label
	if label == "" {
		label = "Working…"
	}
	clock := fmt.Sprintf("%02d:%02d", int(elapsed.Seconds())/60, int(elapsed.Seconds())%60)
	if width < 56 || height < 14 {
		label = ansi.Truncate(label, max(1, width-2), "…")
		if color && marker == "" {
			label = shimmer(label, elapsed)
		}
		second := l.spinner.View() + " " + clock
		if l.transferActive && height < 12 {
			second = ansi.Truncate(l.transferDetail(), max(1, width), "…")
		}
		result := "> " + label + "\n" + second
		if l.transferActive && height >= 12 {
			result += "\n" + l.transferView(max(1, width), color, true)
		}
		if activity := l.activityView(width, height-2, color); activity != "" {
			result += "\n" + activity
		}
		return result
	}
	sideBySide := width >= 96 && height >= 19
	textWidth := min(48, width)
	if sideBySide {
		textWidth = min(48, width-46)
	}
	label = ansi.Truncate(label, textWidth-2, "…")
	if color && marker == "" {
		label = shimmer(label, elapsed)
	}
	dim := func(s string) string {
		if color {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("#78939F")).Render(s)
		}
		return s
	}
	title := "PROVISIONING"
	if len(l.future) == 0 && len(l.completed) == 0 {
		title = "WORKING"
	}
	buildDetails := func(transferMeter bool) string {
		timeline := []string{dim(title), "", "> " + label, dim("  " + clock + " elapsed"), ""}
		if bar := l.stepBar(textWidth, color, marker != ""); bar != "" {
			timeline = append(timeline, bar, "")
		}
		if l.transferActive {
			timeline = append(timeline, l.transferView(textWidth, color, transferMeter), "")
		}
		for _, done := range l.completed {
			timeline = append(timeline, ansi.Truncate("✓ "+done, textWidth, "…"))
		}
		for _, step := range l.future {
			timeline = append(timeline, dim(ansi.Truncate("○ "+step, textWidth, "…")))
		}
		if activity := l.activityView(textWidth, min(6, height-lipgloss.Height(strings.Join(timeline, "\n"))-1), color); activity != "" {
			timeline = append(timeline, "", activity)
		}
		return strings.Join(timeline, "\n")
	}
	details := buildDetails(false)
	mode, ratio := l.forgeState()
	wave := l.now.Sub(l.activityChanged)
	if mode == forgeMeasured && l.transferCompleted && !l.transferCompletedAt.IsZero() {
		wave = max(time.Duration(0), l.now.Sub(l.transferCompletedAt))
	}
	motif := provisioningForge(mode, ratio, l.forgeElapsed(), wave, color, marker)
	if marker == "" && l.motifKind() != "measured" {
		motif = stageMotif(l.motifKind(), l.forgeElapsed(), color)
	}
	if sideBySide {
		return lipgloss.JoinHorizontal(lipgloss.Center, motif, "     ", lipgloss.NewStyle().Width(textWidth).Render(details))
	}
	// Keep the timeline readable on medium terminals; omit the motif before
	// dropping real state. Small terminals keep the existing two-line spinner.
	if height >= 19+2+lipgloss.Height(details) {
		return lipgloss.JoinVertical(lipgloss.Left, motif, "", details)
	}
	return buildDetails(true)
}

func shimmer(label string, elapsed time.Duration) string {
	chars := []rune(label)
	phase := (elapsed % (2400 * time.Millisecond)).Seconds()
	head := phase/1.6*float64(len(chars)+8) - 4
	var b strings.Builder
	for i, r := range chars {
		color := "#59B9CA"
		distance := math.Abs(float64(i) - head)
		if phase < 1.6 && distance < 1.2 {
			color = "#F1FFFF"
		} else if phase < 1.6 && distance < 3 {
			color = "#9EF3FF"
		}
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(string(r)))
	}
	return b.String()
}

// Errors freeze the same visual identity without scheduling an animation.
func (l provisioningLoader) frozen(width, height int, color bool, marker string) string {
	if width < 56 || height < 28 {
		return ""
	}
	mode, ratio := l.forgeState()
	motif := provisioningForge(mode, ratio, 0, time.Second, color, marker)
	bar := l.stepBar(min(width, 48), color, true)
	if bar != "" {
		motif += "\n" + lipgloss.PlaceHorizontal(lipgloss.Width(motif), lipgloss.Center, bar)
	}
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, motif) + "\n"
}

func (l provisioningLoader) forgeState() (forgeMode, float64) {
	if l.transferTotal > 0 || l.transferRatioHigh > 0 || l.transferCompleted {
		return forgeMeasured, l.transferRatioHigh
	}
	return forgeIntake, 0
}
