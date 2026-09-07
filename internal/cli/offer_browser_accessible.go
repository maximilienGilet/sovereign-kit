package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

func (p *AccessiblePrompter) BrowseOffers(ctx context.Context, r recipe.Recipe, search setup.OfferSearch) (vast.Offer, error) {
	query := setup.OfferQuery{Sort: vast.SortPrice}
	var result setup.OfferSearchResult
	refresh := true
	for {
		if err := ctx.Err(); err != nil {
			return vast.Offer{}, err
		}
		if refresh {
			var err error
			result, err = search(ctx, query)
			if err != nil {
				if ctx.Err() != nil {
					return vast.Offer{}, ctx.Err()
				}
				if _, writeErr := fmt.Fprintln(p.output, "Search failed:", cleanOfferText(err.Error())); writeErr != nil {
					return vast.Offer{}, writeErr
				}
				result.Views = nil
			}
			refresh = false
		}
		countries := "All countries"
		if len(query.Countries) > 0 {
			countries = strings.Join(query.Countries, ",")
		}
		if query.Region != "" {
			countries = vast.RegionName(query.Region) + " · " + countries
		}
		if _, err := fmt.Fprintf(p.output, "\n%s · %s · sort %s · %d eligible / %d loaded (limit %d)\n", cleanOfferText(r.Name), countries, query.Sort, len(result.Views), result.Loaded, result.Limit); err != nil {
			return vast.Offer{}, err
		}
		if result.Limit > 0 && result.Loaded >= result.Limit {
			if _, err := fmt.Fprintln(p.output, "Result limit reached; refine countries to search more precisely."); err != nil {
				return vast.Offer{}, err
			}
		}
		for i, view := range result.Views {
			if _, err := fmt.Fprintf(p.output, "%d. %s× %s · %s · %s · %s\n", i+1, providerInt(view.Offer.GPUCount), cleanOfferText(view.Offer.GPUName), offerCountry(view.Offer), offerPrice(view.Offer), offerReliability(view.Offer)); err != nil {
				return vast.Offer{}, err
			}
		}
		if len(result.Views) == 0 {
			if _, err := fmt.Fprintln(p.output, "No eligible offers. Change filters or refresh."); err != nil {
				return vast.Offer{}, err
			}
		}
		value, err := p.inputValue(ctx, "Number to review, g region, f countries, s sort, r refresh, i inspect, q cancel", "", false)
		if err != nil {
			return vast.Offer{}, err
		}
		switch strings.ToLower(value) {
		case "q":
			return vast.Offer{}, context.Canceled
		case "r":
			refresh = true
		case "s":
			if query.Sort == vast.SortPrice {
				query.Sort = vast.SortReliability
			} else {
				query.Sort = vast.SortPrice
			}
			refresh = true
		case "g":
			regions := vast.Regions()
			for i, region := range regions {
				if _, err := fmt.Fprintf(p.output, "%d. %s\n", i, region.Name); err != nil {
					return vast.Offer{}, err
				}
			}
			value, err := p.inputValue(ctx, "Region number", "", false)
			if err != nil {
				return vast.Offer{}, err
			}
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 || n >= len(regions) {
				if _, err := fmt.Fprintln(p.output, "Choose an available region number."); err != nil {
					return vast.Offer{}, err
				}
				continue
			}
			query.Region = regions[n].ID
			catalog, _ := vast.RegionCountries(query.Region)
			allowed := make(map[string]bool, len(catalog))
			for _, country := range catalog {
				allowed[country.Code] = true
			}
			var retained []string
			for _, code := range query.Countries {
				if allowed[code] {
					retained = append(retained, code)
				}
			}
			query.Countries = retained
			refresh = true
		case "f":
			codes, err := p.inputValue(ctx, "Country ISO codes separated by commas, or all", "all", false)
			if err != nil {
				return vast.Offer{}, err
			}
			var normalized []string
			if strings.ToLower(codes) != "all" {
				normalized, err = vast.NormalizeCountries(strings.Split(codes, ","))
			}
			if err == nil {
				_, err = vast.GeographicCountries(query.Region, normalized)
			}
			if err != nil {
				if _, writeErr := fmt.Fprintln(p.output, cleanOfferText(err.Error())); writeErr != nil {
					return vast.Offer{}, writeErr
				}
				continue
			}
			query.Countries = normalized
			refresh = true
		case "i":
			index, err := p.inputValue(ctx, "Offer number to inspect", "", false)
			if err != nil {
				return vast.Offer{}, err
			}
			n, err := strconv.Atoi(index)
			if err != nil || n < 1 || n > len(result.Views) {
				if _, err := fmt.Fprintln(p.output, "Choose an available offer number."); err != nil {
					return vast.Offer{}, err
				}
				continue
			}
			if _, err := fmt.Fprintln(p.output, ansi.Strip(offerFacts(result.Views[n-1], 80, 1))+"\n"+offerInspection(result.Views[n-1])); err != nil {
				return vast.Offer{}, err
			}
		default:
			n, err := strconv.Atoi(value)
			if err == nil && n >= 1 && n <= len(result.Views) {
				return result.Views[n-1].Offer, nil
			}
			if _, err := fmt.Fprintln(p.output, "Choose an available offer number or command."); err != nil {
				return vast.Offer{}, err
			}
		}
	}
}

// Standalone adapter only. The root uses applicationPrompter and never starts a
// second terminal program. Text mode shares the injected stream-safe reader.
func (p *HuhPrompter) BrowseOffers(ctx context.Context, r recipe.Recipe, search setup.OfferSearch) (vast.Offer, error) {
	if fallback := p.accessible(); fallback != nil {
		return fallback.BrowseOffers(ctx, r, search)
	}
	browser := newOfferBrowser(ctx, r, search)
	defer browser.Close()
	adapter := &standaloneOfferBrowser{browser: browser}
	_, err := tea.NewProgram(adapter, tea.WithContext(ctx), tea.WithInput(p.Input), tea.WithOutput(p.Output), tea.WithAltScreen()).Run()
	if err != nil {
		return vast.Offer{}, err
	}
	if adapter.selected.ID == 0 {
		return vast.Offer{}, context.Canceled
	}
	return adapter.selected, nil
}

type standaloneOfferBrowser struct {
	browser  *offerBrowserModel
	selected vast.Offer
}

func (m *standaloneOfferBrowser) Init() tea.Cmd { return m.browser.Init() }
func (m *standaloneOfferBrowser) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case browserSelectedMsg:
		m.selected = msg.offer
		return m, tea.Quit
	case browserBackMsg:
		return m, tea.Quit
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" && !m.browser.filtering {
			m.browser.Close()
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		msg.Height = max(1, msg.Height-2)
		_, cmd := m.browser.Update(msg)
		return m, cmd
	}
	_, cmd := m.browser.Update(msg)
	return m, cmd
}
func (m *standaloneOfferBrowser) View() string {
	return m.browser.View() + "\n" + fitBrowser(m.browser.Footer(), m.browser.width, 1)
}
