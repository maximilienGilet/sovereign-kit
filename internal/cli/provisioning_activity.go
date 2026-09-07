package cli

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"strings"
	"time"
)

func (p *applicationPrompter) InstanceActivity(a setup.Activity) {
	s := p.session
	s.mu.Lock()
	s.activity = a
	s.mu.Unlock()
	select {
	case s.events <- a:
	case <-s.ctx.Done():
	}
}

func (m *applicationModel) applyActivity(a setup.Activity) {
	stage := a.Stage
	if stage == "" {
		stage = setup.ProgressWaiting
	}
	if a.InstanceID <= 0 || a.InstanceID != m.instanceID || m.progressStage != stage || (stage != setup.ProgressWaiting && stage != setup.ProgressHostKeys) || a.CheckedAt.IsZero() || a.CheckedAt.Before(m.loader.checkedAt) {
		return
	}
	l := &m.loader
	l.checkedAt = a.CheckedAt
	if l.activityChanged.IsZero() || l.providerStatus != m.sanitize(a.Status) {
		l.activityChanged = a.CheckedAt
	}
	l.providerStatus = m.sanitize(a.Status)
	message := strings.Join(strings.Fields(m.sanitize(a.Message)), " ")
	if message != "" && message != l.providerMessage {
		l.activityChanged = a.CheckedAt
		l.providerMessage = message
		l.activity = append(l.activity, message)
		if len(l.activity) > 4 {
			l.activity = append([]string(nil), l.activity[len(l.activity)-4:]...)
		}
	}
	detailLines := strings.Split(strings.TrimSpace(a.Detail), "\n")
	detailLines = detailLines[max(0, len(detailLines)-3):]
	for i := range detailLines {
		detailLines[i] = ansi.Truncate(m.sanitize(detailLines[i]), 500, "…")
	}
	detail := strings.Join(detailLines, "\n")
	if detail != "" && detail != l.detailSnapshot {
		l.detailSnapshot = detail
		l.activityChanged = a.CheckedAt
		lines := strings.Split(strings.TrimSpace(detail), "\n")
		for _, line := range lines[max(0, len(lines)-3):] {
			line = strings.Join(strings.Fields(line), " ")
			if line != "" && line != message && (len(l.activity) == 0 || l.activity[len(l.activity)-1] != line) {
				l.activity = append(l.activity, line)
			}
		}
		if len(l.activity) > 4 {
			l.activity = append([]string(nil), l.activity[len(l.activity)-4:]...)
		}
	}
}

func (l provisioningLoader) activityView(width, rows int, color bool) string {
	if l.checkedAt.IsZero() || rows < 1 {
		return ""
	}
	ago := max(0, int(l.now.Sub(l.checkedAt)/time.Second))
	status := l.providerStatus
	if status == "" {
		status = "pending"
	}
	header := fmt.Sprintf("Vast · %s · checked %ds ago", status, ago)
	var lines []string
	if rows >= 2 {
		lines = append(lines, ansi.Truncate(header, width, "…"))
	}
	if rows >= 3 && !l.activityChanged.IsZero() {
		lines = append(lines, ansi.Truncate(fmt.Sprintf("last change %ds ago", max(0, int(l.now.Sub(l.activityChanged)/time.Second))), width, "…"))
	}
	count := min(len(l.activity), max(1, rows-len(lines)))
	for i := len(l.activity) - count; i < len(l.activity); i++ {
		prefix := "  "
		tone := "#78939F"
		if i == len(l.activity)-1 {
			prefix = "› "
			tone = "#B9F4FF"
		}
		line := ansi.Truncate(prefix+l.activity[i], width, "…")
		if color {
			line = lipgloss.NewStyle().Foreground(lipgloss.Color(tone)).Render(line)
		}
		lines = append(lines, line)
	}
	if len(l.activity) == 0 {
		lines = append(lines, ansi.Truncate("› Awaiting provider detail", width, "…"))
	}
	return strings.Join(lines, "\n")
}

// Accessible output is append-only; unlike the animation it contains only
// actual successful provider observations, sanitized before crossing this API.
func (p *AccessiblePrompter) InstanceActivity(a setup.Activity) {
	fmt.Fprintf(p.output, "Vast #%d · %s · %s\n", a.InstanceID, cleanOfferText(a.Status), cleanOfferText(a.Message))
}
