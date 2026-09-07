package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

const vastBalanceRefresh = 60 * time.Second
const vastBalanceTimeout = 3 * time.Second

type vastBalanceResult struct {
	amount            float64
	err               error
	credentialMissing bool
}

type vastBalanceTick struct{}
type vastBalanceStopped struct{}

func (m *applicationModel) readAvailableVastToken() string {
	if token := strings.TrimSpace(m.deps.Setup.Getenv("VAST_API_KEY")); token != "" {
		return token
	}
	if m.deps.Setup.Credentials == nil {
		return ""
	}
	token, err := m.deps.Setup.Credentials.Load()
	if err != nil && !os.IsNotExist(err) {
		return ""
	}
	return strings.TrimSpace(token)
}

func (m *applicationModel) readVastBalance() tea.Cmd {
	token := m.readAvailableVastToken()
	if token == "" {
		if !m.balanceStarted {
			return nil
		}
		return func() tea.Msg { return vastBalanceResult{credentialMissing: true} }
	}
	m.balanceReading = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.balanceCtx, vastBalanceTimeout)
		defer cancel()
		amount, err := m.deps.Balance(ctx, token)
		return vastBalanceResult{amount: amount, err: err}
	}
}

func (m *applicationModel) waitVastBalance() tea.Cmd {
	m.balanceScheduled = true
	ctx := m.balanceCtx
	return func() tea.Msg {
		timer := time.NewTimer(vastBalanceRefresh)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return vastBalanceStopped{}
		case <-timer.C:
			return vastBalanceTick{}
		}
	}
}

func (m *applicationModel) reconcileVastBalance() tea.Cmd {
	if m.balanceStarted {
		return nil
	}
	command := m.readVastBalance()
	if command == nil {
		return nil
	}
	m.balanceStarted = true
	return command
}

func (m *applicationModel) applicationHeader(title string) string {
	if m.vastBalance == nil {
		return m.ink(ansi.Truncate(title, m.width, ""), "#60DBC0")
	}
	right := fmt.Sprintf("VAST  $%.2f", *m.vastBalance)
	rightWidth := ansi.StringWidth(right)
	if rightWidth >= m.width {
		return m.ink(ansi.Truncate(right, m.width, ""), "#60DBC0")
	}
	leftWidth := m.width - rightWidth - 1
	left := ansi.Truncate(title, leftWidth, "…")
	gap := strings.Repeat(" ", m.width-ansi.StringWidth(left)-rightWidth)
	return m.ink(left, "#60DBC0") + gap + m.ink(right, "#60DBC0")
}

func defaultVastBalance(ctx context.Context, token string) (float64, error) {
	return vast.NewClient("https://console.vast.ai", token).Balance(ctx)
}
