package planner

import (
	"testing"

	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

func TestRecommendFiltersInsufficientVRAMAndSortsByCost(t *testing.T) {
	r := recipe.Recipe{Requirements: recipe.Requirements{MinimumVRAMGB: 96}}
	offers := []vast.Offer{
		{ID: 1, GPUName: "RTX 4090", GPUVRAMGB: 24, HourlyUSD: 0.50},
		{ID: 2, GPUName: "RTX PRO 6000", GPUVRAMGB: 96, HourlyUSD: 1.40, Reliability: 0.99},
		{ID: 3, GPUName: "RTX PRO 6000", GPUVRAMGB: 96, HourlyUSD: 1.20, Reliability: 0.98},
	}

	recommendations := Recommend(r, offers, 240)
	if len(recommendations) != 2 {
		t.Fatalf("recommendation count = %d", len(recommendations))
	}
	if recommendations[0].Offer.ID != 3 || recommendations[0].MonthlyUSD != 288 {
		t.Fatalf("unexpected first recommendation: %#v", recommendations[0])
	}
}

func TestRecommendKeepsSpeedUnknownWithoutAnExactReference(t *testing.T) {
	r := recipe.Recipe{ID: "qwen-studio", Requirements: recipe.Requirements{MinimumVRAMGB: 96}}
	recommendations := Recommend(r, []vast.Offer{{ID: 2, GPUName: "RTX PRO 6000", GPUVRAMGB: 96, HourlyUSD: 1.40}}, 100)
	if recommendations[0].Performance.Confidence != Unknown {
		t.Fatalf("performance confidence = %q", recommendations[0].Performance.Confidence)
	}
}
