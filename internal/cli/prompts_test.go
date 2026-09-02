package cli

import (
	"strings"
	"testing"

	"github.com/charmbracelet/huh"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

func TestProviderOptionsUseStableLabels(t *testing.T) {
	options := providerOptions()
	if len(options) != 2 {
		t.Fatalf("expected two providers, got %d", len(options))
	}
	if options[0].Key != "Vast" || options[0].Value != "vast" {
		t.Fatalf("unexpected Vast option: %#v", options[0])
	}
	if options[1].Key != "Manual SSH" || options[1].Value != "manual" {
		t.Fatalf("unexpected manual option: %#v", options[1])
	}
}

func TestOfferLabelsShowCostCapacityAndReliability(t *testing.T) {
	view := setup.OfferView{
		Offer:      vast.Offer{ID: 7, GPUName: "RTX 4090", GPUVRAMGB: 47.5, HourlyUSD: 1.234, Location: "US", Reliability: 0.987},
		MonthlyUSD: 900.82,
		AnnualUSD: 10810.44,
	}
	label := offerLabel(view)
	for _, want := range []string{"RTX 4090", "47.5 GB VRAM", "$1.23/h", "Monthly $900.82", "Annual $10810.44", "US", "98.7%"} {
		if !strings.Contains(label, want) {
			t.Fatalf("offer label missing %q: %s", want, label)
		}
	}
}

func TestOfferLabelsMarkUnknownReliability(t *testing.T) {
	label := offerLabel(setup.OfferView{Offer: vast.Offer{GPUVRAMGB: 0, Reliability: 0}})
	if !strings.Contains(label, "unknown/unmeasured") {
		t.Fatalf("expected unknown reliability marker: %s", label)
	}
}

func TestOfferOptionsUseOfferIDs(t *testing.T) {
	options := offerOptions([]setup.OfferView{{Offer: vast.Offer{ID: 9, GPUName: "A"}}, {Offer: vast.Offer{ID: 3, GPUName: "B"}}})
	if len(options) != 2 || options[0].Value != 9 || options[1].Value != 3 {
		t.Fatalf("unexpected offer options: %#v", options)
	}
}

func TestConfirmationTitlesDescribeIrreversibleActions(t *testing.T) {
	cost := costConfirmationTitle(setup.OfferView{Offer: vast.Offer{GPUName: "A", HourlyUSD: 1}}, 100)
	if !strings.Contains(strings.ToLower(cost), "billing") || !strings.Contains(strings.ToLower(cost), "irreversible") {
		t.Fatalf("cost title lacks billing warning: %s", cost)
	}
	keys := hostKeyConfirmationTitle([]string{"SHA256:abc"})
	if !strings.Contains(strings.ToLower(keys), "first-use") || !strings.Contains(strings.ToLower(keys), "trust") {
		t.Fatalf("host key title lacks trust warning: %s", keys)
	}
}

func TestHuhPrompterImplementsSetupInterfaces(t *testing.T) {
	var _ SetupPrompter = (*HuhPrompter)(nil)
	var _ setup.Operator = (*HuhPrompter)(nil)
	var _ = huh.ErrUserAborted
}
