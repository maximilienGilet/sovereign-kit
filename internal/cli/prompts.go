package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

type HuhPrompter struct {
	Input  io.Reader
	Output io.Writer
}

func NewHuhPrompter(input io.Reader, output io.Writer) *HuhPrompter {
	if input == nil {
		input = os.Stdin
	}
	if output == nil {
		output = os.Stdout
	}
	return &HuhPrompter{Input: input, Output: output}
}

func providerOptions() []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption("Vast", "vast"),
		huh.NewOption("Manual SSH", "manual"),
	}
}

func offerOptions(views []setup.OfferView) []huh.Option[int] {
	options := make([]huh.Option[int], 0, len(views))
	for _, view := range views {
		options = append(options, huh.NewOption(offerLabel(view), view.Offer.ID))
	}
	return options
}

func offerLabel(view setup.OfferView) string {
	gpu := view.Offer.GPUName
	if strings.TrimSpace(gpu) == "" {
		gpu = "Unknown GPU"
	}
	vram := "unknown/unmeasured"
	if view.Offer.GPUVRAMGB > 0 {
		vram = fmt.Sprintf("%g GB VRAM", view.Offer.GPUVRAMGB)
	}
	reliability := "unknown/unmeasured"
	if view.Offer.Reliability > 0 {
		reliability = fmt.Sprintf("%.1f%%", view.Offer.Reliability*100)
	}
	location := strings.TrimSpace(view.Offer.Location)
	if location == "" {
		location = "unknown/unmeasured"
	}
	return fmt.Sprintf("%s · %s · $%.2f/h · Monthly $%.2f · Annual $%.2f · Location: %s · Reliability: %s", gpu, vram, view.Offer.HourlyUSD, view.MonthlyUSD, view.AnnualUSD, location, reliability)
}

func costConfirmationTitle(view setup.OfferView, diskGB int) string {
	return fmt.Sprintf("Create a paid Vast instance at $%.2f/h with %d GB disk? This starts irreversible billing.", view.Offer.HourlyUSD, diskGB)
}

func hostKeyConfirmationTitle(fingerprints []string) string {
	if len(fingerprints) == 0 {
		return "Trust this host fingerprint on first-use? No fingerprint was measured."
	}
	return fmt.Sprintf("Trust this host fingerprint on first-use? %s", strings.Join(fingerprints, ", "))
}

func (p *HuhPrompter) SelectProvider(ctx context.Context) (string, error) {
	var provider string
	field := huh.NewSelect[string]().Title("Choose a setup provider").Options(providerOptions()...).Value(&provider)
	if err := p.run(ctx, field); err != nil {
		return "", err
	}
	return provider, nil
}

func (p *HuhPrompter) ManualRoute(ctx context.Context, defaultUser string) (ManualRoute, error) {
	var route ManualRoute
	portText := "22"
	fields := []huh.Field{
		huh.NewInput().Title("GPU host").Value(&route.Host).Validate(required("GPU host")),
		huh.NewInput().Title("SSH port").Value(&portText).Validate(required("SSH port")),
		huh.NewInput().Title("SSH user").Value(&route.User).Validate(required("SSH user")),
		huh.NewInput().Title("SSH identity file").Value(&route.IdentityFile).Validate(required("SSH identity file")),
		huh.NewInput().Title("Verified known-hosts file").Value(&route.KnownHostsFile).Validate(required("verified known-hosts file")),
		huh.NewInput().Title("SSH user").Value(&route.User),
	}
	route.User = defaultUser
	if err := p.runGroup(ctx, fields...); err != nil {
		return ManualRoute{}, err
	}
	port, err := strconv.Atoi(strings.TrimSpace(portText))
	if err != nil {
		return ManualRoute{}, fmt.Errorf("SSH port must be a number")
	}
	route.Port = port
	return route, nil
}

func (p *HuhPrompter) VastIdentity(ctx context.Context, defaultValue string) (string, error) {
	identity := defaultValue
	field := huh.NewInput().Title("SSH identity file").Value(&identity).Validate(required("SSH identity file"))
	if err := p.run(ctx, field); err != nil {
		return "", err
	}
	return identity, nil
}

func (p *HuhPrompter) SelectOffer(ctx context.Context, views []setup.OfferView) (vast.Offer, error) {
	if len(views) == 0 {
		return vast.Offer{}, fmt.Errorf("no Vast offers available")
	}
	var selected int
	field := huh.NewSelect[int]().Title("Choose a Vast GPU offer").Options(offerOptions(views)...).Value(&selected)
	if err := p.run(ctx, field); err != nil {
		return vast.Offer{}, err
	}
	for _, view := range views {
		if view.Offer.ID == selected {
			return view.Offer, nil
		}
	}
	return vast.Offer{}, fmt.Errorf("selected Vast offer %d is unavailable", selected)
}

func (p *HuhPrompter) ConfirmCost(ctx context.Context, view setup.OfferView, diskGB int) (bool, error) {
	confirmed := false
	field := huh.NewConfirm().Title(costConfirmationTitle(view, diskGB)).Affirmative("Create paid instance").Negative("Cancel").Value(&confirmed)
	if err := p.run(ctx, field); err != nil {
		return false, err
	}
	return confirmed, nil
}

func (p *HuhPrompter) ConfirmHostKeys(ctx context.Context, fingerprints []string) (bool, error) {
	confirmed := false
	field := huh.NewConfirm().Title(hostKeyConfirmationTitle(fingerprints)).Affirmative("Trust this fingerprint").Negative("Cancel").Value(&confirmed)
	if err := p.run(ctx, field); err != nil {
		return false, err
	}
	return confirmed, nil
}

func (p *HuhPrompter) run(ctx context.Context, field huh.Field) error {
	return p.mapFormError(huh.NewForm(huh.NewGroup(field)).WithInput(p.Input).WithOutput(p.Output).RunWithContext(ctx))
}

func (p *HuhPrompter) runGroup(ctx context.Context, fields ...huh.Field) error {
	return p.mapFormError(huh.NewForm(huh.NewGroup(fields...)).WithInput(p.Input).WithOutput(p.Output).RunWithContext(ctx))
}

func (p *HuhPrompter) mapFormError(err error) error {
	if errors.Is(err, huh.ErrUserAborted) {
		return fmt.Errorf("setup cancelled")
	}
	return err
}

func required(label string) func(string) error {
	return func(value string) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", strings.ToLower(label))
		}
		return nil
	}
}
