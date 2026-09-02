package setup

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

type VastAPI interface {
	SearchOffers(context.Context, vast.SearchRequest) ([]vast.Offer, error)
	CreateInstance(context.Context, int, vast.CreateRequest) (int, error)
	GetInstance(context.Context, int) (vast.Instance, error)
}

type Operator interface {
	SelectOffer(context.Context, []OfferView) (vast.Offer, error)
	ConfirmCost(context.Context, OfferView, int) (bool, error)
	ConfirmHostKeys(context.Context, []string) (bool, error)
}

type HostKeys struct {
	Raw          []byte
	Fingerprints []string
}

type HostKeyScanner interface {
	Scan(context.Context, string, int) (HostKeys, error)
}

type TrustStore interface {
	Save(string, []byte) error
}

type ServerLauncher interface {
	Launch(context.Context, config.SSH, recipe.Recipe) error
}

type Clock interface {
	Now() time.Time
	Sleep(context.Context, time.Duration) error
}

type Dependencies struct {
	NewAPI           func(string) VastAPI
	Operator         Operator
	HostKeyScanner   HostKeyScanner
	TrustStore       TrustStore
	ServerLauncher   ServerLauncher
	Clock            Clock
	SaveConfig       func(string, config.Config) error
	ValidateIdentity func(string) error
}

type Options struct {
	ConfigPath    string
	IdentityFile  string
	KnownHostsDir string
	OfferLimit    int
	PollInterval  time.Duration
	PollTimeout   time.Duration
}

type OfferView struct {
	Offer      vast.Offer
	MonthlyUSD float64
	AnnualUSD  float64
}

type Result struct {
	InstanceID int
	ConfigPath string
}

func RunVast(ctx context.Context, token string, r recipe.Recipe, options Options, deps Dependencies) (Result, error) {
	if strings.TrimSpace(token) == "" {
		return Result{}, fmt.Errorf("Vast API key is required")
	}
	if err := r.Validate(); err != nil {
		return Result{}, err
	}
	if deps.ValidateIdentity == nil {
		return Result{}, fmt.Errorf("identity validator is required")
	}
	if err := deps.ValidateIdentity(options.IdentityFile); err != nil {
		return Result{}, err
	}
	if deps.NewAPI == nil {
		return Result{}, fmt.Errorf("Vast API constructor is required")
	}
	api := deps.NewAPI(strings.TrimSpace(token))
	if api == nil {
		return Result{}, fmt.Errorf("Vast API is required")
	}
	if deps.Operator == nil {
		return Result{}, fmt.Errorf("setup operator is required")
	}
	offers, err := api.SearchOffers(ctx, vast.SearchRequest{Limit: options.OfferLimit, MinimumVRAMGB: r.Requirements.MinimumVRAMGB})
	if err != nil {
		return Result{}, err
	}
	eligible := make([]vast.Offer, 0, len(offers))
	for _, offer := range offers {
		if offer.GPUVRAMGB >= float64(r.Requirements.MinimumVRAMGB) {
			eligible = append(eligible, offer)
		}
	}
	if len(eligible) == 0 {
		return Result{}, fmt.Errorf("no eligible Vast offers found")
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		if eligible[i].HourlyUSD == eligible[j].HourlyUSD {
			return eligible[i].ID < eligible[j].ID
		}
		return eligible[i].HourlyUSD < eligible[j].HourlyUSD
	})
	views := make([]OfferView, len(eligible))
	for i, offer := range eligible {
		views[i] = viewFor(offer)
	}
	selected, err := deps.Operator.SelectOffer(ctx, views)
	if err != nil {
		return Result{}, err
	}
	selectedView, ok := selectedView(views, selected.ID)
	if !ok {
		return Result{}, fmt.Errorf("selected Vast offer %d is not eligible", selected.ID)
	}
	confirmed, err := deps.Operator.ConfirmCost(ctx, selectedView, r.Requirements.MinimumDiskGB)
	if err != nil {
		return Result{}, err
	}
	if !confirmed {
		return Result{}, fmt.Errorf("Vast setup cancelled: cost was not confirmed")
	}
	if deps.Clock == nil {
		return Result{}, fmt.Errorf("clock is required")
	}
	if deps.HostKeyScanner == nil || deps.TrustStore == nil || deps.ServerLauncher == nil || deps.SaveConfig == nil {
		return Result{}, fmt.Errorf("post-create setup dependencies are required")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	instanceID, err := api.CreateInstance(ctx, selectedView.Offer.ID, vast.CreateRequest{Image: r.Runtime.Image, DiskGB: r.Requirements.MinimumDiskGB, Label: "sovkit-" + r.ID})
	if err != nil {
		return Result{}, err
	}
	instance, err := waitForRunning(ctx, api, instanceID, options, deps.Clock)
	if err != nil {
		return Result{}, paidInstanceError(instanceID, err)
	}
	knownHostsPath := filepath.Join(options.KnownHostsDir, fmt.Sprintf("vast-%d_known_hosts", instanceID))
	if err := ctx.Err(); err != nil {
		return Result{}, paidInstanceError(instanceID, err)
	}
	keys, err := deps.HostKeyScanner.Scan(ctx, instance.SSHHost, instance.SSHPort)
	if err != nil {
		return Result{}, paidInstanceError(instanceID, err)
	}
	if len(bytes.TrimSpace(keys.Raw)) == 0 || !hasFingerprint(keys.Fingerprints) {
		return Result{}, paidInstanceError(instanceID, fmt.Errorf("Vast host-key scan returned no usable key material"))
	}
	if err := ctx.Err(); err != nil {
		return Result{}, paidInstanceError(instanceID, err)
	}
	confirmed, err = deps.Operator.ConfirmHostKeys(ctx, keys.Fingerprints)
	if err != nil {
		return Result{}, paidInstanceError(instanceID, err)
	}
	if !confirmed {
		return Result{}, paidInstanceError(instanceID, fmt.Errorf("Vast host-key confirmation was declined"))
	}
	if err := ctx.Err(); err != nil {
		return Result{}, paidInstanceError(instanceID, err)
	}
	if err := deps.TrustStore.Save(knownHostsPath, keys.Raw); err != nil {
		return Result{}, paidInstanceError(instanceID, err)
	}
	ssh := config.SSH{Host: instance.SSHHost, Port: instance.SSHPort, User: "root", IdentityFile: options.IdentityFile, KnownHostsFile: knownHostsPath}
	if err := ctx.Err(); err != nil {
		return Result{}, paidInstanceError(instanceID, err)
	}
	if err := deps.ServerLauncher.Launch(ctx, ssh, r); err != nil {
		return Result{}, paidInstanceError(instanceID, err)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, paidInstanceError(instanceID, err)
	}
	if err := deps.SaveConfig(options.ConfigPath, config.VastStudio(instanceID, instance.SSHHost, instance.SSHPort, options.IdentityFile, knownHostsPath)); err != nil {
		return Result{}, paidInstanceError(instanceID, err)
	}
	return Result{InstanceID: instanceID, ConfigPath: options.ConfigPath}, nil
}

func viewFor(offer vast.Offer) OfferView {
	return OfferView{Offer: offer, MonthlyUSD: offer.HourlyUSD * 730, AnnualUSD: offer.HourlyUSD * 8760}
}

func selectedView(views []OfferView, id int) (OfferView, bool) {
	for _, view := range views {
		if view.Offer.ID == id {
			return view, true
		}
	}
	return OfferView{}, false
}

func waitForRunning(ctx context.Context, api VastAPI, instanceID int, options Options, clock Clock) (vast.Instance, error) {
	deadline := clock.Now().Add(options.PollTimeout)
	for {
		if err := ctx.Err(); err != nil {
			return vast.Instance{}, err
		}
		instance, err := api.GetInstance(ctx, instanceID)
		if err != nil {
			return vast.Instance{}, err
		}
		status := strings.ToLower(strings.TrimSpace(instance.Status))
		switch status {
		case "exited", "unknown", "offline":
			return vast.Instance{}, fmt.Errorf("Vast instance has terminal status %q", instance.Status)
		case "running":
			if strings.TrimSpace(instance.SSHHost) != "" && instance.SSHPort > 0 {
				return instance, nil
			}
		default:
			if status != "loading" && status != "creating" && status != "queued" && status != "starting" {
				return vast.Instance{}, fmt.Errorf("Vast instance has unusable status %q", instance.Status)
			}
		}
		if !clock.Now().Before(deadline) {
			return vast.Instance{}, fmt.Errorf("timed out waiting for Vast instance to become ready")
		}
		if options.PollInterval <= 0 {
			return vast.Instance{}, fmt.Errorf("poll interval must be positive")
		}
		if err := clock.Sleep(ctx, options.PollInterval); err != nil {
			return vast.Instance{}, err
		}
	}
}

func paidInstanceError(instanceID int, err error) error {
	return fmt.Errorf("Vast instance %d was created; billing may still be active: %w", instanceID, err)
}

func hasFingerprint(fingerprints []string) bool {
	for _, fingerprint := range fingerprints {
		if strings.TrimSpace(fingerprint) != "" {
			return true
		}
	}
	return false
}
