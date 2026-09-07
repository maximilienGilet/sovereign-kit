package setup

import (
	"context"
	"slices"
	"testing"

	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

func TestRegionSearchExpandsServerCountriesAndRejectsIncompatibleRefinement(t *testing.T) {
	api := &fakeVastAPI{offers: []vast.Offer{
		{ID: 1, GPUVRAMGB: 96, Location: "Paris, FR"},
		{ID: 2, GPUVRAMGB: 96, Location: "US"},
	}}
	result, err := SearchEligibleOffers(context.Background(), api, validRecipe(), OfferQuery{Region: "europe"}, 100)
	if err != nil || len(result.Views) != 1 || result.Views[0].Offer.ID != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if !slices.Contains(api.searchReq.Countries, "FR") || !slices.Contains(api.searchReq.Countries, "DE") || slices.Contains(api.searchReq.Countries, "US") {
		t.Fatalf("wrong server region: %v", api.searchReq.Countries)
	}
	for _, query := range []OfferQuery{{Region: "invalid"}, {Region: "europe", Countries: []string{"US"}}} {
		called := false
		guarded := &searchAPI{search: func(context.Context, vast.SearchRequest) ([]vast.Offer, error) { called = true; return nil, nil }}
		if _, err := SearchEligibleOffers(context.Background(), guarded, validRecipe(), query, 100); err == nil || called {
			t.Fatalf("invalid region searched: %+v err=%v called=%v", query, err, called)
		}
	}
	_, err = SearchEligibleOffers(context.Background(), api, validRecipe(), OfferQuery{Region: "europe", Countries: []string{"fr"}}, 100)
	if err != nil || len(api.searchReq.Countries) != 1 || api.searchReq.Countries[0] != "FR" {
		t.Fatalf("refinement=%v err=%v", api.searchReq.Countries, err)
	}
}
