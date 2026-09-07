package cli

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestOfferFactsShowPerGPUCapacityAndUnknowns(t *testing.T) {
	view := rentalReviewFixture()
	view.Offer.GPUCount = 2
	view.Offer.GPUVRAMGB = 48
	view.Offer.TotalGPUVRAMGB = 96
	view.Recipe.Requirements.MinimumVRAMGB = 32
	view.Offer.PriceUnknown = true
	view.Offer.ReliabilityUnknown = true
	view.Offer.Location = "Paris\x1b[2J\r, FR"
	facts := ansi.Strip(offerFacts(view, 70, 1))
	for _, want := range []string{"48 GB / GPU", "96 GB total", "32 GB required", "Price unknown", "Reliability unknown", "│"} {
		if !strings.Contains(facts, want) {
			t.Fatalf("missing %q:\n%s", want, facts)
		}
	}
	if strings.Contains(facts, "$0.00") || strings.Contains(facts, "RADAR") || strings.Contains(facts, "\r") {
		t.Fatalf("misleading/unsafe facts: %q", facts)
	}
}

func TestOfferCapacityUsesDeclaredScaleAndRequirementMarker(t *testing.T) {
	bar := ansi.Strip(capacityBar("VRAM", 48, 32, "GB / GPU", 40, 1))
	if !strings.Contains(bar, "48") || !strings.Contains(bar, "32 GB required") || !strings.Contains(bar, "0") || !strings.Contains(bar, "│") {
		t.Fatalf("missing scale: %s", bar)
	}
	if unknown := capacityBar("Disk", 0, 100, "GB / instance", 40, 1); strings.Contains(unknown, "█") || !strings.Contains(unknown, "unknown") {
		t.Fatalf("unknown plotted: %s", unknown)
	}
}

func TestRentalReviewUnknownPriceAndReliabilityNeverInventValues(t *testing.T) {
	view := rentalReviewFixture()
	view.Offer.PriceUnknown = true
	view.Offer.ReliabilityUnknown = true
	summary := newRentalReview(view, 120).accessibleSummary()
	if !strings.Contains(summary, "Price unknown") || !strings.Contains(summary, "Reliability unknown") || strings.Contains(summary, "$1.50") || strings.Contains(summary, "99.9%") {
		t.Fatalf("unknown data treated as measured: %s", summary)
	}
}

func TestOfferPriceRetainsNonCentPrecisionAndRulerEndpointsAlign(t *testing.T) {
	view := rentalReviewFixture()
	if got := offerPrice(view.Offer); got != "$1.5037/h" {
		t.Errorf("rounded provider quote: %s", got)
	}
	bar := strings.Split(ansi.Strip(capacityBar("VRAM", 97.9, 96, "GB / GPU", 40, 1)), "\n")
	if len(bar) < 4 {
		t.Fatal("ruler legend must be separate from endpoint labels")
	}
	if ansi.StringWidth(bar[1]) != ansi.StringWidth(bar[2]) {
		t.Errorf("bar/ruler endpoints differ: %q / %q", bar[1], bar[2])
	}
}
