package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
)

func TestOfferRegionDraftApplyDiscardAndCountryScope(t *testing.T) {
	var got setup.OfferQuery
	m := newOfferBrowser(context.Background(), testRecipe(), func(_ context.Context, q setup.OfferQuery) (setup.OfferSearchResult, error) {
		got = q
		return browserFixture(), nil
	})
	defer m.Close()
	m.query.Countries = []string{"FR", "US"}
	browserKey(m, "f")
	for range 3 {
		m.Update(tea.KeyMsg{Type: tea.KeyRight})
	}
	if !strings.Contains(m.View(), "Europe") {
		t.Fatalf("region control absent: %s", m.View())
	}
	seenFR := false
	for _, c := range m.filteredCountries() {
		if c.Code == "US" {
			t.Fatal("US visible in Europe")
		}
		if c.Code == "FR" {
			seenFR = true
		}
	}
	if !seenFR {
		t.Fatal("country catalog not available independently of offers")
	}
	browserKey(m, "esc")
	if m.query.Region != "" || len(m.query.Countries) != 2 {
		t.Fatal("escape changed committed query")
	}
	browserKey(m, "f")
	for range 3 {
		m.Update(tea.KeyMsg{Type: tea.KeyRight})
	}
	cmd := browserKey(m, "enter")
	m.Update(cmd())
	if got.Region != "europe" {
		t.Fatalf("region not sent to search: %+v", got)
	}
	for _, c := range got.Countries {
		if c == "US" {
			t.Fatal("incompatible country retained")
		}
	}
	browserKey(m, "f")
	browserKey(m, " ") // All countries means all within this region.
	cmd = browserKey(m, "enter")
	m.Update(cmd())
	if got.Region != "europe" || len(got.Countries) != 0 {
		t.Fatalf("all countries escaped region: %+v", got)
	}
}

func TestAccessibleRegionFiltersPreserveCountryCommandAndRejectMismatch(t *testing.T) {
	var output bytes.Buffer
	var queries []setup.OfferQuery
	p := NewAccessiblePrompter(strings.NewReader("g\n3\nf\nUS\nf\nFR\n1\n"), &output)
	_, err := p.BrowseOffers(context.Background(), testRecipe(), func(_ context.Context, q setup.OfferQuery) (setup.OfferSearchResult, error) {
		queries = append(queries, q)
		return browserFixture(), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 3 || queries[1].Region != "europe" || queries[2].Region != "europe" || len(queries[2].Countries) != 1 || queries[2].Countries[0] != "FR" {
		t.Fatalf("queries=%+v output=%s", queries, output.String())
	}
	if !strings.Contains(output.String(), "outside") {
		t.Fatal("incompatible country was not explained")
	}
}
