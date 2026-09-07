package cli

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

type browserSelectedMsg struct{ offer vast.Offer }
type browserBackMsg struct{}
type browserSearchMsg struct {
	generation uint64
	result     setup.OfferSearchResult
	err        error
}
type browserTickMsg struct{ generation uint64 }

// Only this read-only search capability crosses the service/UI boundary.
type offerBrowserModel struct {
	ctx                                                   context.Context
	search                                                setup.OfferSearch
	recipe                                                recipe.Recipe
	cancel                                                context.CancelFunc
	generation, animation                                 uint64
	frame                                                 int
	query                                                 setup.OfferQuery
	views                                                 []setup.OfferView
	selectedID, loaded, limit                             int
	width, height                                         int
	loading, closed, details, inspection, filtering, help bool
	errText                                               string
	loadingFrame                                          string
	filter                                                textinput.Model
	countryDraft                                          map[string]bool
	regionDraft                                           string
	countryIndex                                          int
	viewport                                              viewport.Model
}

func newOfferBrowser(ctx context.Context, r recipe.Recipe, search setup.OfferSearch) *offerBrowserModel {
	filter := textinput.New()
	filter.Prompt = "/ "
	filter.Placeholder = "Search countries"
	filter.CharLimit = 60
	filter.Focus()
	return &offerBrowserModel{ctx: ctx, recipe: r, search: search, query: setup.OfferQuery{Sort: vast.SortPrice}, width: 80, height: 20, filter: filter, viewport: viewport.New(78, 18), frame: 4}
}
func (m *offerBrowserModel) Init() tea.Cmd { return m.refresh() }
func (m *offerBrowserModel) Close() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.closed = true
}
func (m *offerBrowserModel) refresh() tea.Cmd {
	if m.cancel != nil {
		m.cancel()
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	m.generation++
	id := m.generation
	query := m.query
	query.Countries = append([]string(nil), query.Countries...)
	m.loading, m.errText = true, ""
	m.views = nil
	m.viewport.GotoTop()
	search := m.search
	return func() tea.Msg {
		result, err := search(ctx, query)
		return browserSearchMsg{id, result, err}
	}
}
func (m *offerBrowserModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.closed {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = max(1, min(146, msg.Width)), max(1, msg.Height)
		m.viewport.Width, m.viewport.Height = max(1, m.width-2), max(1, m.height-1)
		m.filter.Width = max(1, m.width-4)
		m.updateDetail()
	case browserSearchMsg:
		if msg.generation != m.generation {
			return m, nil
		}
		m.loading = false
		m.views, m.loaded, m.limit = msg.result.Views, msg.result.Loaded, msg.result.Limit
		if msg.err != nil {
			m.errText = cleanOfferText(msg.err.Error())
			m.views = nil
		}
		if m.selectedIndex() < 0 {
			m.selectedID = 0
			if len(m.views) > 0 {
				m.selectedID = m.views[0].Offer.ID
			}
		}
		m.updateDetail()
		return m, m.animate()
	case browserTickMsg:
		if msg.generation == m.animation && m.frame < 4 {
			m.frame++
			if m.frame < 4 {
				return m, m.tick()
			}
		}
	case tea.KeyMsg:
		if m.filtering {
			return m, m.updateFilter(msg)
		}
		if m.help {
			if msg.String() == "esc" || msg.String() == "?" {
				m.help = false
			} else {
				m.viewport, _ = m.viewport.Update(msg)
			}
			return m, nil
		}
		switch msg.String() {
		case "?":
			m.help = true
			m.viewport.GotoTop()
			m.viewport.SetContent("↑/↓ Move offer\nf Region / countries\ns Sort price/reliability\nr Refresh\ni Full inspection\nEnter Details / choose\nEsc Close details / back\nq Quit application\nPgUp/PgDn Scroll details")
		case "f":
			m.filtering = true
			m.regionDraft = m.query.Region
			m.filter.SetValue("")
			m.countryIndex = 0
			m.countryDraft = make(map[string]bool)
			for _, code := range m.query.Countries {
				m.countryDraft[code] = true
			}
		case "s":
			if m.query.Sort == vast.SortReliability {
				m.query.Sort = vast.SortPrice
			} else {
				m.query.Sort = vast.SortReliability
			}
			return m, m.refresh()
		case "r":
			return m, m.refresh()
		case "i":
			if m.selectedIndex() >= 0 {
				m.details, m.inspection = true, true
				m.viewport.GotoTop()
				m.updateDetail()
			}
		case "esc":
			if m.details {
				m.details, m.inspection = false, false
				m.viewport.GotoTop()
				return m, nil
			}
			m.Close()
			return m, func() tea.Msg { return browserBackMsg{} }
		case "enter":
			index := m.selectedIndex()
			if m.loading || index < 0 {
				return m, nil
			}
			if m.width < 108 && !m.details {
				m.details = true
				m.viewport.GotoTop()
				m.updateDetail()
				return m, nil
			}
			offer := m.views[index].Offer
			m.Close()
			return m, func() tea.Msg { return browserSelectedMsg{offer} }
		case "up", "down":
			if m.details {
				m.viewport, _ = m.viewport.Update(msg)
				return m, nil
			}
			index := m.selectedIndex()
			if index < 0 || m.loading {
				return m, nil
			}
			if msg.String() == "up" {
				index = max(0, index-1)
			} else {
				index = min(len(m.views)-1, index+1)
			}
			m.selectedID = m.views[index].Offer.ID
			m.updateDetail()
			return m, m.animate()
		default:
			if m.details || m.width >= 108 {
				m.viewport, _ = m.viewport.Update(msg)
			}
		}
	}
	return m, nil
}
func (m *offerBrowserModel) selectedIndex() int {
	for i, view := range m.views {
		if view.Offer.ID == m.selectedID {
			return i
		}
	}
	return -1
}
func (m *offerBrowserModel) animate() tea.Cmd {
	m.animation++
	m.frame = 0
	return m.tick()
}
func (m *offerBrowserModel) tick() tea.Cmd {
	id := m.animation
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg { return browserTickMsg{id} })
}
func (m *offerBrowserModel) filteredCountries() []vast.Country {
	query := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	catalog, _ := vast.RegionCountries(m.regionDraft)
	all := append([]vast.Country{{Name: "All countries"}}, catalog...)
	var result []vast.Country
	for _, country := range all {
		if query == "" || strings.Contains(strings.ToLower(country.Code+" "+country.Name), query) {
			result = append(result, country)
		}
	}
	return result
}
func (m *offerBrowserModel) updateFilter(msg tea.KeyMsg) tea.Cmd {
	countries := m.filteredCountries()
	switch msg.String() {
	case "esc":
		m.filtering = false
		return nil
	case "enter":
		var codes []string
		for code, selected := range m.countryDraft {
			if selected {
				codes = append(codes, code)
			}
		}
		sort.Strings(codes)
		m.query.Countries, _ = vast.NormalizeCountries(codes)
		m.query.Region = m.regionDraft
		m.filtering = false
		return m.refresh()
	case "left", "right":
		regions := vast.Regions()
		index := 0
		for i, region := range regions {
			if region.ID == m.regionDraft {
				index = i
				break
			}
		}
		if msg.String() == "left" {
			index = (index + len(regions) - 1) % len(regions)
		} else {
			index = (index + 1) % len(regions)
		}
		m.regionDraft = regions[index].ID
		catalog, _ := vast.RegionCountries(m.regionDraft)
		allowed := make(map[string]bool, len(catalog))
		for _, country := range catalog {
			allowed[country.Code] = true
		}
		for code := range m.countryDraft {
			if !allowed[code] {
				delete(m.countryDraft, code)
			}
		}
		m.countryIndex = 0
		m.filter.SetValue("")
	case "up":
		m.countryIndex = max(0, m.countryIndex-1)
	case "down":
		m.countryIndex = min(max(0, len(countries)-1), m.countryIndex+1)
	case " ":
		if len(countries) == 0 {
			return nil
		}
		code := countries[m.countryIndex].Code
		if code == "" {
			m.countryDraft = make(map[string]bool)
		} else {
			m.countryDraft[code] = !m.countryDraft[code]
		}
	default:
		m.filter, _ = m.filter.Update(msg)
		m.countryIndex = 0
	}
	return nil
}
