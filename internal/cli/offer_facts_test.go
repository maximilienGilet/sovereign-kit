package cli

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

func offerFactsFixture() setup.OfferView {
	return setup.OfferView{
		Offer: vast.Offer{
			ID: 42, MachineID: 88, GPUName: "RTX PRO 6000 S", GPUCount: 1, GPUVRAMGB: 97.9, TotalGPUVRAMGB: 97.9,
			CPUCores: 48, CPURAMGB: 257.75, DiskSpaceGB: 1251.32500000000005, InetDownMBps: 7494.8, InetUpMBps: 5233,
			DriverVersion: "570.86.15", HourlyUSD: 1.5037, Location: "Maryland, US", Reliability: 0.999,
		},
		Recipe: recipe.Recipe{
			Name: "Qwen Studio", Kind: "text-generation",
			Profile:      recipe.Profile{Status: "reference", Summary: "Long-context private Qwen route", Evidence: "Historical synthetic evidence"},
			Runtime:      recipe.Runtime{Engine: "sglang", Image: "lmsysorg/sglang@sha256:e21dd539b36ea7842101393ec3fe3b0d453626cd8251e4d4db33af0cd97d7f0b"},
			Model:        recipe.Model{Repository: "RadixArk/Qwen3.8-27B-NVFP4", Revision: "319f741cce68d7914884900c138a1fbb70a42f30"},
			Serve:        recipe.Serve{ContextWindow: 262144, MaxOutputTokens: 16384, MaxRunningRequests: 5},
			Requirements: recipe.Requirements{GPUModel: "RTX PRO 6000 S", GPUCount: 1, StrictGPU: true, MinimumVRAMGB: 96, MinimumDiskGB: 120},
		},
		MonthlyUSD: 1097.70,
		AnnualUSD:  13172.41,
	}
}

func TestOfferFactsShowPerGPUCapacityAndUnknowns(t *testing.T) {
	view := offerFactsFixture()
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

func TestOfferPriceRetainsNonCentPrecisionAndRulerEndpointsAlign(t *testing.T) {
	view := offerFactsFixture()
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
