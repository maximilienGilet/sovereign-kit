package cli

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"strings"
)

func (p *applicationPrompter) ServerLogSnapshot(logs setup.ServerLogs) {
	s := p.session
	s.mu.Lock()
	s.logs = logs
	s.mu.Unlock()
	select {
	case s.events <- logs:
	case <-s.ctx.Done():
	}
}

func (p *AccessiblePrompter) ServerLogSnapshot(logs setup.ServerLogs) {
	if transfer := setup.ParseDownloadProgress(logs.Text); transfer != nil {
		detail := transferBytes(transfer.Current) + " · total unknown"
		if transfer.Total > 0 && transfer.Current <= transfer.Total {
			detail = fmt.Sprintf("%s / %s — %.1f%%", transferBytes(transfer.Current), transferBytes(transfer.Total), float64(transfer.Current)/float64(transfer.Total)*100)
		}
		if detail != p.lastDownload {
			fmt.Fprintln(p.output, "Model download: "+detail)
			p.lastDownload = detail
		}
	}
	lines := strings.Split(logs.Text, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "SOVKIT_DOWNLOAD ") {
			kept = append(kept, line)
		}
	}
	text := cleanOfferText(strings.Join(kept, "\n"))
	if text != "" && text != p.lastServerLogs {
		fmt.Fprintln(p.output, "Server logs:\n"+text)
		p.lastServerLogs = text
	}
}

func (m *applicationModel) applyServerLogs(logs setup.ServerLogs) {
	if logs.CheckedAt.IsZero() || logs.CheckedAt.Before(m.serverLogs.CheckedAt) {
		return
	}
	lines := strings.Split(logs.Text, "\n")
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(m.sanitize(lines[i]), 500, "…")
	}
	logs.Text = strings.Join(lines, "\n")
	follow := m.logViewport.AtBottom() || m.serverLogs.CheckedAt.IsZero()
	m.serverLogs = logs
	if m.progressStage == setup.ProgressLaunching {
		for _, line := range lines {
			switch {
			case strings.Contains(line, "SOVKIT_DOWNLOAD "):
				m.loader.serverPhase = "download"
			case strings.Contains(line, "Verifying GGUF SHA256"):
				m.loader.serverPhase = "inspection"
			case strings.Contains(line, "Launching llama-server"), strings.Contains(line, "load_model:"):
				m.loader.serverPhase = "activation"
			}
		}
		if measured := setup.ParseDownloadHighWater(logs.Text); measured != nil {
			ratio := float64(measured.Current) / float64(measured.Total)
			m.loader.transferRatioHigh = max(m.loader.transferRatioHigh, ratio)
			if ratio >= 1 && !m.loader.transferCompleted {
				m.loader.transferCompleted = true
				m.loader.transferCompletedAt = logs.CheckedAt
			}
		}
		transfer := setup.ParseDownloadProgress(logs.Text)
		m.loader.transferActive = transfer != nil
		if transfer != nil {
			m.loader.transferCurrent, m.loader.transferTotal = transfer.Current, transfer.Total
			if transfer.Total > 0 && transfer.Current <= transfer.Total {
				ratio := float64(transfer.Current) / float64(transfer.Total)
				m.loader.transferRatioHigh = max(m.loader.transferRatioHigh, ratio)
				if ratio >= 1 && !m.loader.transferCompleted {
					m.loader.transferCompleted = true
					m.loader.transferCompletedAt = logs.CheckedAt
				}
			}
		}
	}
	m.logViewport.SetContent(ansi.Wrap(logs.Text, max(1, m.logViewport.Width), ""))
	if follow {
		m.logViewport.GotoBottom()
	}
}

func (m *applicationModel) compactServerLogs(width, rows int) string {
	var lines []string
	for _, line := range strings.Split(m.serverLogs.Text, "\n") {
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "SOVKIT_DOWNLOAD ") {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		lines = []string{"Waiting for server log output…"}
		if setup.ParseDownloadProgress(m.serverLogs.Text) != nil {
			lines = []string{"Downloading model weights · progress shown above"}
		}
	}
	count := min(len(lines), max(1, rows-1))
	result := []string{"SERVER LOGS · l expand"}
	for _, line := range lines[len(lines)-count:] {
		result = append(result, ansi.Truncate(line, max(1, width), "…"))
	}
	return strings.Join(result, "\n")
}

func (m *applicationModel) expandedServerLogs() string {
	m.logViewport.Width = max(1, m.width)
	m.logViewport.Height = max(1, m.height-7)
	atBottom := m.logViewport.AtBottom()
	text := m.serverLogs.Text
	if text == "" {
		text = "Waiting for server log output…"
	}
	m.logViewport.SetContent(ansi.Wrap(text, m.logViewport.Width, ""))
	if atBottom {
		m.logViewport.GotoBottom()
	}
	return m.applicationHeader("SOVEREIGN KIT / SERVER LOGS") + "\n" + m.compactPaidWarning() + "\n" + m.logViewport.View() + "\n↑↓ PgUp/PgDn scroll · l/Esc back · Ctrl+C stop"
}
