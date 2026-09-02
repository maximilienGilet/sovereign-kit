package cli

import (
	"context"
	"errors"
	"github.com/charmbracelet/huh"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
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
		AnnualUSD:  10810.44,
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
	for _, want := range []string{"$1.00/h", "100 GB disk", "billing", "irreversible", "Compute only; excludes storage, egress, and tax."} {
		if !strings.Contains(cost, want) {
			t.Fatalf("cost title missing %q: %s", want, cost)
		}
	}
	keys := hostKeyConfirmationTitle([]string{"SHA256:abc"})
	if !strings.Contains(strings.ToLower(keys), "first-use") || !strings.Contains(strings.ToLower(keys), "trust") {
		t.Fatalf("host key title lacks trust warning: %s", keys)
	}
}

func TestOfferLabelsNameComputePeriods(t *testing.T) {
	label := offerLabel(setup.OfferView{Offer: vast.Offer{HourlyUSD: 1}, MonthlyUSD: 730, AnnualUSD: 8760})
	for _, want := range []string{"730h monthly compute", "8,760h annual compute", "Monthly $730.00", "Annual $8760.00"} {
		if !strings.Contains(label, want) {
			t.Fatalf("offer label missing %q: %s", want, label)
		}
	}
}

func TestMapFormErrorMapsAbortAndPassesThrough(t *testing.T) {
	prompter := &HuhPrompter{}
	if err := prompter.mapFormError(huh.ErrUserAborted); err == nil || err.Error() != "setup cancelled" {
		t.Fatalf("abort error = %v, want setup cancelled", err)
	}
	other := errors.New("other")
	if err := prompter.mapFormError(other); err != other {
		t.Fatalf("other error = %v, want original error", err)
	}
}

func TestSetupRejectsUnknownProvider(t *testing.T) {
	err := Setup(context.Background(), strings.NewReader(""), &strings.Builder{}, "/tmp/config.toml", "alice", SetupDependencies{
		Prompter: &fakeSetupPrompter{provider: "unknown"},
	})
	if err == nil || !strings.Contains(err.Error(), `unknown setup provider "unknown"`) {
		t.Fatalf("error = %v, want unknown provider rejection", err)
	}
}

func TestSetupManualRejectsOutOfRangePorts(t *testing.T) {
	for _, port := range []int{0, 65536} {
		t.Run(strconv.Itoa(port), func(t *testing.T) {
			dir := t.TempDir()
			identity := filepath.Join(dir, "identity")
			knownHosts := filepath.Join(dir, "known_hosts")
			if err := os.WriteFile(identity, []byte("key"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(knownHosts, []byte("host ssh-ed25519 AAAA"), 0o600); err != nil {
				t.Fatal(err)
			}
			err := setupManual(context.Background(), &strings.Builder{}, filepath.Join(dir, "config.toml"), "alice", &fakeSetupPrompter{
				manual: ManualRoute{Host: "gpu", Port: port, User: "alice", IdentityFile: identity, KnownHostsFile: knownHosts},
			})
			if err == nil || !strings.Contains(err.Error(), "between 1 and 65535") {
				t.Fatalf("error = %v, want port bounds rejection", err)
			}
		})
	}
}

func TestHuhPrompterImplementsSetupInterfaces(t *testing.T) {
	var _ SetupPrompter = (*HuhPrompter)(nil)
	var _ setup.Operator = (*HuhPrompter)(nil)
	var _ = huh.ErrUserAborted
}

func TestManualRouteFieldsHaveOnePrefilledSSHUser(t *testing.T) {
	route := ManualRoute{}
	portText := "22"
	fields := manualRouteFields(&route, &portText, "alice")
	if len(fields) != 5 {
		t.Fatalf("expected five manual route inputs, got %d", len(fields))
	}
	if route.User != "alice" {
		t.Fatalf("expected default SSH user, got %q", route.User)
	}
	if got := fields[2].GetValue(); got != "alice" {
		t.Fatalf("expected prefilled SSH user field, got %#v", got)
	}
	if view := fields[2].View(); !strings.Contains(view, "alice") {
		t.Fatalf("SSH user view = %q, want alice", view)
	}
	count := 0
	for _, field := range fields {
		count += strings.Count(field.View(), "SSH user")
	}
	if count != 1 {
		t.Fatalf("expected one SSH user prompt, got %d", count)
	}
}
