package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

func (m *offerBrowserModel) Footer() string {
	if m.filtering {
		if m.width < 48 {
			return "←→ region · Space Enter Esc"
		}
		return "←→ region · Type search · ↑↓ country · Space toggle · Enter apply · Esc discard"
	}
	if m.help {
		return "↑↓/PgDn scroll · Esc close"
	}
	if m.details {
		return "Enter choose · PgDn · Esc back"
	}
	if m.width < 48 {
		return "f s r i Enter Esc · ? help"
	}
	if m.width >= 80 && m.width < 108 {
		return "↑↓ · f location · s sort · r refresh · i inspect · Enter details · Esc back · ?"
	}
	if m.width < 108 {
		return "↑↓ · f location · s sort · r refresh · i · Enter · Esc · ?"
	}
	return "↑↓ offers · f location · s sort · r refresh · i inspect · Enter choose · Esc back · ? help"
}
func (m *offerBrowserModel) queryLine() string {
	countries := "All countries"
	if len(m.query.Countries) > 0 {
		countries = strings.Join(m.query.Countries, ",")
	}
	if m.query.Region != "" {
		countries = vast.RegionName(m.query.Region) + " · " + countries
	}
	sort := "price ↑"
	if strings.Contains(string(m.query.Sort), "reliability") {
		sort = "reliability ↓"
	}
	if m.width < 48 {
		name := ansi.Truncate(cleanOfferText(m.recipe.Name), max(1, m.width-ansi.StringWidth(sort)-3), "…")
		line := fmt.Sprintf("%d eligible/%d loaded", len(m.views), m.loaded)
		if m.limit > 0 && m.loaded >= m.limit {
			line += " refine"
		} else {
			line += " · " + countries
		}
		return name + " · " + sort + "\n" + line
	}
	limit := ""
	if m.limit > 0 && m.loaded >= m.limit {
		limit = " · limit reached: refine countries"
	}
	return cleanOfferText(m.recipe.Name) + " · " + sort + "\n" + fmt.Sprintf("%s · %d eligible / %d loaded (limit %d)%s", countries, len(m.views), m.loaded, m.limit, limit)
}
func (m *offerBrowserModel) updateDetail() {
	index := m.selectedIndex()
	if index < 0 || m.help {
		return
	}
	width := max(1, m.width-2)
	height := max(1, m.height-lipgloss.Height(m.queryLine()))
	if m.width >= 108 && !m.details {
		width = m.width - 48 - 4
		height = max(1, m.height-lipgloss.Height(m.queryLine())-2)
	}
	m.viewport.Width, m.viewport.Height = width, height
	content := offerFacts(m.views[index], width, float64(m.frame)/4)
	if m.inspection {
		content += "\n\n" + offerInspection(m.views[index])
	}
	m.viewport.SetContent(ansi.Wrap(content, width, ""))
}
func (m *offerBrowserModel) View() string {
	var body string
	switch {
	case m.filtering:
		body = m.filterView()
	case m.help:
		body = m.viewport.View()
	case m.loading:
		body = m.queryLine() + "\n\n" + m.loadingFrame + " Searching offers…\nFilters, sort and refresh remain available."
	case m.errText != "":
		body = m.queryLine() + "\n\nSearch failed: " + m.errText + "\nChange countries or retry with r."
	case len(m.views) == 0:
		body = m.queryLine() + "\n\nNo eligible offers.\nChange countries, sort or refresh."
	case m.details:
		m.updateDetail()
		body = m.queryLine() + "\n" + m.viewport.View()
	case m.width >= 108:
		// Render animated bars from current state; labels never interpolate.
		m.updateDetail()
		panelHeight := max(1, m.height-lipgloss.Height(m.queryLine())-2)
		left := browserPanel(m.offerRows(44, panelHeight), 44, panelHeight)
		right := browserPanel(m.viewport.View(), m.width-52, panelHeight)
		body = m.queryLine() + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
	default:
		body = m.queryLine() + "\n" + m.offerRows(m.width, max(1, m.height-lipgloss.Height(m.queryLine())))
	}
	return fitBrowser(body, m.width, m.height)
}
func browserPanel(content string, width, height int) string {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(rentalLine).Width(width).Height(height).Render(fitBrowser(content, width, height))
}
func fitBrowser(content string, width, height int) string {
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], max(1, width), "…")
	}
	return strings.Join(lines, "\n")
}
func (m *offerBrowserModel) offerRows(width, height int) string {
	index := max(0, m.selectedIndex())
	rowHeight := 2
	if width < 48 {
		rowHeight = 3
	}
	count := max(1, height/rowHeight)
	start := max(0, min(index-count/2, len(m.views)-count))
	var rows []string
	for i := start; i < min(len(m.views), start+count); i++ {
		o := m.views[i].Offer
		cursor := "  "
		if i == index {
			cursor = "> "
		}
		title := fmt.Sprintf("%s%s× %s", cursor, providerInt(o.GPUCount), cleanOfferText(o.GPUName))
		if i == index {
			title = rentalAccent.Render(ansi.Truncate(title, width, "…"))
		}
		rows = append(rows, title)
		if width < 48 {
			rows = append(rows, rentalWarning.Render(offerPrice(o))+" · "+offerCountry(o), offerReliability(o))
		} else {
			country := vast.CountryCode(o.Location)
			if country == "" {
				country = "Country unknown"
			}
			reliability := strings.Replace(offerReliability(o), "Reliability ", "rel ", 1)
			rows = append(rows, rentalWarning.Render(offerPrice(o))+" · "+country+" · "+reliability)
		}
	}
	return fitBrowser(strings.Join(rows, "\n"), width, height)
}
func (m *offerBrowserModel) filterView() string {
	countries := m.filteredCountries()
	selected := 0
	for _, on := range m.countryDraft {
		if on {
			selected++
		}
	}
	lines := []string{rentalAccent.Render("Region  ‹ " + vast.RegionName(m.regionDraft) + " ›"), fmt.Sprintf("Countries · %d selected", selected), m.filter.View()}
	count := max(1, m.height-4)
	start := max(0, min(m.countryIndex-count/2, len(countries)-count))
	for i := start; i < min(len(countries), start+count); i++ {
		country := countries[i]
		mark, cursor := "[ ]", "  "
		if m.countryDraft[country.Code] || country.Code == "" && selected == 0 {
			mark = "[x]"
		}
		if i == m.countryIndex {
			cursor = "> "
		}
		line := cursor + mark + " " + country.Name
		if country.Code != "" {
			line += " (" + country.Code + ")"
		}
		if i == m.countryIndex {
			line = rentalAccent.Render(ansi.Truncate(line, m.width, "…"))
		}
		lines = append(lines, line)
	}
	if len(countries) == 0 {
		lines = append(lines, "No matching countries")
	}
	return strings.Join(lines, "\n")
}
