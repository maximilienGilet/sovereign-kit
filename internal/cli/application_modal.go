package cli

import "github.com/maximilienGilet/sovereign-kit/internal/terminalui"

func (m *applicationModel) modal(background, content string) string {
	return terminalui.Modal(background, content, m.width, m.height-4, m.deps.Setup.Getenv("NO_COLOR") == "")
}
