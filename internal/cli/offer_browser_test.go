package cli

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

func browserFixture() setup.OfferSearchResult {
	first := rentalReviewFixture()
	first.Offer.Location = "Paris, FR"
	second := first
	second.Offer.ID = 43
	second.Offer.GPUName = "RTX 6000 Ada"
	second.Offer.Location = "Berlin, DE"
	second.Offer.HourlyUSD = 1.25
	second.MonthlyUSD = 912.5
	return setup.OfferSearchResult{Views: []setup.OfferView{first, second}, Loaded: 2, Limit: 100}
}
func browserKey(m *offerBrowserModel, key string) tea.Cmd {
	var msg tea.KeyMsg
	switch key {
	case "enter":
		msg.Type = tea.KeyEnter
	case "esc":
		msg.Type = tea.KeyEsc
	case "down":
		msg.Type = tea.KeyDown
	case "up":
		msg.Type = tea.KeyUp
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	_, cmd := m.Update(msg)
	return cmd
}

func TestOfferBrowserCompactHeaderShowsEligibleAndLoadedCounts(t *testing.T) {
	for _, loaded := range []int{4, 100} {
		m := newOfferBrowser(context.Background(), testRecipe(), func(context.Context, setup.OfferQuery) (setup.OfferSearchResult, error) {
			result := browserFixture()
			result.Views = result.Views[:1]
			result.Loaded = loaded
			return result, nil
		})
		m.Update(tea.WindowSizeMsg{Width: 30, Height: 6})
		m.Update(m.Init()())
		view := m.View()
		wantLoaded := "4 loaded"
		if loaded == 100 {
			wantLoaded = "100 loaded"
		}
		if !strings.Contains(view, "1 eligible") || !strings.Contains(view, wantLoaded) {
			t.Errorf("compact header hides search coverage: %s", view)
		}
		if loaded == 100 && !strings.Contains(view, "refine") {
			t.Errorf("compact capped result lacks refinement invitation: %s", view)
		}
		m.Close()
	}
}
func TestOfferBrowserSearchGatesSelectionAndRejectsLateResults(t *testing.T) {
	var queries []setup.OfferQuery
	m := newOfferBrowser(context.Background(), testRecipe(), func(_ context.Context, q setup.OfferQuery) (setup.OfferSearchResult, error) {
		queries = append(queries, q)
		return browserFixture(), nil
	})
	defer m.Close()
	m.Update(tea.WindowSizeMsg{Width: 146, Height: 30})
	initial := m.Init()
	if cmd := browserKey(m, "enter"); cmd != nil {
		t.Fatal("loading selected an offer")
	}
	old := initial()
	refresh := browserKey(m, "r")
	m.Update(old)
	if !m.loading || len(m.views) != 0 {
		t.Fatal("late response replaced current search")
	}
	m.Update(refresh())
	if m.loading || m.selectedID != 42 {
		t.Fatal("fresh result not displayed")
	}
	browserKey(m, "down")
	if m.selectedID != 43 {
		t.Fatal("navigation did not select next row")
	}
	sort := browserKey(m, "s")
	if cmd := browserKey(m, "enter"); cmd != nil {
		t.Fatal("sorting selected stale offer")
	}
	m.Update(sort())
	if len(queries) != 3 || queries[2].Sort != vast.SortReliability || m.selectedID != 43 {
		t.Fatalf("sort/selection not retained: %+v", queries)
	}
	cmd := browserKey(m, "enter")
	if selected, ok := cmd().(browserSelectedMsg); !ok || selected.offer.ID != 43 {
		t.Fatal("explicit selection returned wrong offer")
	}
	if browserKey(m, "enter") != nil {
		t.Fatal("duplicate selection emitted")
	}
}
func TestOfferBrowserCountryDraftCancelAndEmptyErrorRetainQuery(t *testing.T) {
	m := newOfferBrowser(context.Background(), testRecipe(), func(context.Context, setup.OfferQuery) (setup.OfferSearchResult, error) {
		return setup.OfferSearchResult{}, errors.New("temporary outage")
	})
	defer m.Close()
	m.query.Countries = []string{"FR", "DE"}
	browserKey(m, "f")
	browserKey(m, "Japan")
	browserKey(m, " ")
	browserKey(m, "esc")
	if !reflect.DeepEqual(m.query.Countries, []string{"FR", "DE"}) {
		t.Fatal("cancel applied filter draft")
	}
	browserKey(m, "f")
	browserKey(m, "Japan")
	browserKey(m, " ")
	cmd := browserKey(m, "enter")
	m.Update(cmd())
	if !reflect.DeepEqual(m.query.Countries, []string{"DE", "FR", "JP"}) || m.errText == "" {
		t.Fatalf("error lost country union: %+v", m.query)
	}
	m.Update(browserSearchMsg{generation: m.generation, result: setup.OfferSearchResult{Limit: 100}})
	if len(m.query.Countries) != 3 || m.selectedID != 0 {
		t.Fatal("empty result lost query or retained selection")
	}
	if browserKey(m, "enter") != nil {
		t.Fatal("empty result selected")
	}
	browserKey(m, "f")
	if len(m.filteredCountries()) < 200 {
		t.Fatal("country catalog derived from empty offers")
	}
}
func TestOfferBrowserNarrowDetailsDoNotSelectAndCloseCancels(t *testing.T) {
	var searchCtx context.Context
	m := newOfferBrowser(context.Background(), testRecipe(), func(ctx context.Context, _ setup.OfferQuery) (setup.OfferSearchResult, error) {
		searchCtx = ctx
		return browserFixture(), nil
	})
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 6})
	m.Update(m.Init()())
	if cmd := browserKey(m, "enter"); cmd != nil || !m.details {
		t.Fatal("narrow Enter must inspect before choosing")
	}
	if browserKey(m, "esc") != nil || m.details {
		t.Fatal("details Esc left browser")
	}
	browserKey(m, "f")
	browserKey(m, "q")
	if !strings.Contains(m.filter.Value(), "q") {
		t.Fatal("filter swallowed q")
	}
	browserKey(m, "esc")
	back := browserKey(m, "esc")
	if _, ok := back().(browserBackMsg); !ok || searchCtx.Err() == nil {
		t.Fatal("closing failed to cancel browser search")
	}
}

func TestOfferBrowserAnimationStopsAndMissingSelectionIsNotRetained(t *testing.T) {
	result := browserFixture()
	m := newOfferBrowser(context.Background(), testRecipe(), func(context.Context, setup.OfferQuery) (setup.OfferSearchResult, error) { return result, nil })
	defer m.Close()
	_, tick := m.Update(m.Init()())
	for i := 0; i < 4; i++ {
		_, tick = m.Update(browserTickMsg{m.animation})
	}
	if tick != nil || m.frame != 4 {
		t.Fatal("animation did not finish")
	}
	browserKey(m, "down")
	oldAnimation := m.animation
	result.Views = result.Views[:1]
	m.Update(browserKey(m, "r")())
	if m.selectedID != 42 {
		t.Fatal("missing selected ID retained")
	}
	m.Update(browserTickMsg{oldAnimation})
	if m.frame != 0 {
		t.Fatal("late animation advanced replacement selection")
	}
	browserKey(m, "f")
	m.countryDraft = map[string]bool{"FR": true, "DE": true}
	browserKey(m, " ")
	m.Update(browserKey(m, "enter")())
	if len(m.query.Countries) != 0 {
		t.Fatal("All countries did not reset union")
	}
}
