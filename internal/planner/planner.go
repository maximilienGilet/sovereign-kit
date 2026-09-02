// Package planner turns recipe requirements and live provider offers into a
// transparent hardware shortlist. It deliberately does not invent performance.
package planner

import (
	"sort"

	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

type Confidence string

const Unknown Confidence = "unknown"

type Performance struct {
	Confidence Confidence
	Reason     string
}

type Recommendation struct {
	Offer       vast.Offer
	MonthlyUSD  float64
	Performance Performance
}

// Recommend filters offers that cannot meet the recipe's declared VRAM floor,
// orders the remainder by visible hourly price, and labels speed as unknown
// until an exact matching benchmark or a local probe exists.
func Recommend(recipe recipe.Recipe, offers []vast.Offer, monthlyHours float64) []Recommendation {
	recommendations := make([]Recommendation, 0, len(offers))
	for _, offer := range offers {
		if offer.GPUVRAMGB < float64(recipe.Requirements.MinimumVRAMGB) {
			continue
		}
		recommendations = append(recommendations, Recommendation{
			Offer:      offer,
			MonthlyUSD: offer.HourlyUSD * monthlyHours,
			Performance: Performance{
				Confidence: Unknown,
				Reason:     "No exact benchmark or local probe for this recipe and offer",
			},
		})
	}
	sort.SliceStable(recommendations, func(left, right int) bool {
		return recommendations[left].Offer.HourlyUSD < recommendations[right].Offer.HourlyUSD
	})
	return recommendations
}
