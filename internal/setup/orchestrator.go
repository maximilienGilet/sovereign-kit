package setup

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
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
	PrepareIdentity  func(context.Context) error
	ValidateIdentity func(string) error
}

type Options struct {
	CheckpointPath   string
	Resume           bool
	ResumeInstanceID int
	RecoveryOnly     bool
	ConfigPath       string
	IdentityFile     string
	KnownHostsDir    string
	AutoSelectOffer  bool
	OfferLimit       int
	PollInterval     time.Duration
	PollTimeout      time.Duration
}

type OfferView struct {
	Offer      vast.Offer
	Recipe     recipe.Recipe
	MonthlyUSD float64
	AnnualUSD  float64
}

type Result struct {
	InstanceID int
	ConfigPath string
}

func RunVast(ctx context.Context, token string, r recipe.Recipe, options Options, deps Dependencies) (Result, error) {
	if options.CheckpointPath != "" {
		return runDurableVast(ctx, token, r, options, deps)
	}
	if options.Resume || options.ResumeInstanceID != 0 || options.RecoveryOnly {
		return Result{}, fmt.Errorf("resume requires a checkpoint path")
	}
	if strings.TrimSpace(token) == "" {
		return Result{}, fmt.Errorf("Vast API key is required")
	}
	if err := r.Validate(); err != nil {
		return Result{}, err
	}
	if deps.ValidateIdentity == nil {
		return Result{}, fmt.Errorf("identity validator is required")
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
	notifyProgress(deps.Operator, ProgressSearching, 0)
	selectedOfferView, err := chooseEligibleOffer(ctx, api, r, options.OfferLimit, deps.Operator, token)
	if err != nil {
		return Result{}, err
	}
	if selectedOfferView.Offer.PriceUnknown {
		return Result{}, fmt.Errorf("selected Vast offer has unknown price and cannot be rented")
	}
	confirmed, err := deps.Operator.ConfirmCost(ctx, selectedOfferView, r.Requirements.MinimumDiskGB)
	if err != nil {
		return Result{}, err
	}
	if !confirmed {
		return Result{}, fmt.Errorf("Vast setup cancelled: cost was not confirmed")
	}
	if deps.PrepareIdentity != nil {
		if err := deps.PrepareIdentity(ctx); err != nil {
			return Result{}, err
		}
	}
	if err := deps.ValidateIdentity(options.IdentityFile); err != nil {
		return Result{}, err
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
	notifyProgress(deps.Operator, ProgressCreating, 0)
	instanceID, err := api.CreateInstance(ctx, selectedOfferView.Offer.ID, vast.CreateRequest{Image: r.Runtime.Image, DiskGB: r.Requirements.MinimumDiskGB, Label: "sovkit-" + r.ID})
	if err != nil {
		return Result{}, err
	}
	if instanceID <= 0 {
		return Result{}, fmt.Errorf("Vast did not return a positive instance ID")
	}
	return finishVast(ctx, token, r, options, deps, api, instanceID, nil)
}

func finishVast(ctx context.Context, token string, r recipe.Recipe, options Options, deps Dependencies, api VastAPI, instanceID int, checkpoint *Checkpoint) (Result, error) {
	if destroyer, ok := api.(InstanceDestroyer); ok {
		notifyRecovery(deps.Operator, newInstanceRecovery(instanceID, destroyer, options, deps.Clock))
	}
	notifyProgress(deps.Operator, ProgressCreated, instanceID)
	notifyProgress(deps.Operator, ProgressWaiting, instanceID)
	instance, err := waitForRunning(ctx, api, instanceID, options, deps.Clock, func(instance vast.Instance) {
		notifyActivity(deps.Operator, instance, instanceID, deps.Clock.Now(), token)
	})
	if err != nil {
		return Result{}, paidInstanceError(instanceID, err)
	}
	knownHostsPath := filepath.Join(options.KnownHostsDir, fmt.Sprintf("vast-%d_known_hosts", instanceID))
	if err := ctx.Err(); err != nil {
		return Result{}, paidInstanceError(instanceID, err)
	}
	notifyProgress(deps.Operator, ProgressHostKeys, instanceID)
	keys, err := waitForHostKeys(ctx, instance.SSHHost, instance.SSHPort, instanceID, deps)
	if err != nil {
		return Result{}, paidInstanceError(instanceID, err)
	}
	if len(bytes.TrimSpace(keys.Raw)) == 0 || !hasFingerprint(keys.Fingerprints) {
		return Result{}, paidInstanceError(instanceID, fmt.Errorf("Vast host-key scan returned no usable key material"))
	}
	if err := ctx.Err(); err != nil {
		return Result{}, paidInstanceError(instanceID, err)
	}
	confirmed := false
	if checkpoint != nil {
		confirmed = checkpoint.trustMatches(instance, knownHostsPath, keys.Raw)
	}
	if !confirmed {
		confirmed, err = deps.Operator.ConfirmHostKeys(ctx, keys.Fingerprints)
	}
	if err != nil {
		return Result{}, paidInstanceError(instanceID, err)
	}
	if !confirmed {
		return Result{}, paidInstanceError(instanceID, fmt.Errorf("Vast host-key confirmation was declined"))
	}
	if err := ctx.Err(); err != nil {
		return Result{}, paidInstanceError(instanceID, err)
	}
	// Reused trust is already persisted; raw keyscan ordering is not stable.
	trustReused := checkpoint != nil && checkpoint.trustMatches(instance, knownHostsPath, keys.Raw)
	if !trustReused {
		if err := deps.TrustStore.Save(knownHostsPath, keys.Raw); err != nil {
			return Result{}, paidInstanceError(instanceID, err)
		}
	}
	if checkpoint != nil {
		checkpoint.ApprovedHost, checkpoint.ApprovedPort, checkpoint.KnownHostsFile = instance.SSHHost, instance.SSHPort, knownHostsPath
		checkpoint.HostKeysHash, checkpoint.Phase = hostKeysDigest(keys.Raw), "trusted"
		if err := writeCheckpoint(options.CheckpointPath, *checkpoint); err != nil {
			return Result{}, paidInstanceError(instanceID, err)
		}
	}
	ssh := config.SSH{Host: instance.SSHHost, Port: instance.SSHPort, User: "root", IdentityFile: options.IdentityFile, KnownHostsFile: knownHostsPath}
	if err := ctx.Err(); err != nil {
		return Result{}, paidInstanceError(instanceID, err)
	}
	notifyProgress(deps.Operator, ProgressLaunching, instanceID)
	var launchErr error
	if checkpoint != nil {
		checkpoint.Phase = "launch-intent"
		if err := writeCheckpoint(options.CheckpointPath, *checkpoint); err != nil {
			return Result{}, paidInstanceError(instanceID, err)
		}
		if reconciler, ok := deps.ServerLauncher.(ServerReconciler); ok {
			launchErr = reconciler.Reconcile(ctx, ssh, r)
		} else {
			launchErr = fmt.Errorf("safe server reconciliation is required for persistent deployments")
		}
	} else {
		launchErr = deps.ServerLauncher.Launch(ctx, ssh, r)
	}
	if launchErr != nil {
		return Result{}, paidInstanceError(instanceID, launchErr)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, paidInstanceError(instanceID, err)
	}
	notifyProgress(deps.Operator, ProgressSaving, instanceID)
	cfg := config.VastStudio(instanceID, instance.SSHHost, instance.SSHPort, options.IdentityFile, knownHostsPath)
	cfg.Model = config.Model{ID: r.Model.Repository, ContextWindow: r.Serve.ContextWindow, MaxTokens: r.Serve.MaxOutputTokens}
	if err := deps.SaveConfig(options.ConfigPath, cfg); err != nil {
		return Result{}, paidInstanceError(instanceID, err)
	}
	if checkpoint != nil {
		if err := removeCheckpoint(options.CheckpointPath); err != nil {
			return Result{}, paidInstanceError(instanceID, err)
		}
	}
	return Result{InstanceID: instanceID, ConfigPath: options.ConfigPath}, nil
}

func viewFor(offer vast.Offer, selectedRecipe recipe.Recipe) OfferView {
	return OfferView{Offer: offer, Recipe: selectedRecipe, MonthlyUSD: offer.HourlyUSD * 730, AnnualUSD: offer.HourlyUSD * 8760}
}

func selectedView(views []OfferView, id int) (OfferView, bool) {
	for _, view := range views {
		if view.Offer.ID == id {
			return view, true
		}
	}
	return OfferView{}, false
}

func waitForRunning(ctx context.Context, api VastAPI, instanceID int, options Options, clock Clock, observed func(vast.Instance)) (vast.Instance, error) {
	deadline := clock.Now().Add(options.PollTimeout)
	for {
		if err := ctx.Err(); err != nil {
			return vast.Instance{}, err
		}
		instance, err := api.GetInstance(ctx, instanceID)
		if err != nil {
			return vast.Instance{}, err
		}
		if options.CheckpointPath != "" && instance.ID != instanceID {
			return vast.Instance{}, fmt.Errorf("provider returned a different instance; refusing SSH connection")
		}
		if observed != nil {
			observed(instance)
		}
		status := strings.ToLower(strings.TrimSpace(instance.Status))
		switch status {
		case "exited", "unknown", "offline":
			return vast.Instance{}, fmt.Errorf("Vast instance has terminal status %q", instance.Status)
		case "running":
			if strings.TrimSpace(instance.SSHHost) != "" && instance.SSHPort > 0 {
				return instance, nil
			}
		case "", "loading", "creating", "queued", "starting":
		default:
			return vast.Instance{}, fmt.Errorf("Vast instance has unusable status %q", instance.Status)
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
