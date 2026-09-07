// Package provision rents one Vast instance from a recipe and prepares it
// to serve: identity, keys, host pinning, server launch. Every transition
// persists; a rented-but-broken instance is destroyed, never orphaned.
package provision

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/state"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

// VastAPI is the instance lifecycle surface provision drives.
type VastAPI interface {
	CreateInstance(ctx context.Context, offerID int, request vast.CreateRequest) (int, error)
	GetInstance(ctx context.Context, id int) (vast.Instance, error)
	DestroyInstance(ctx context.Context, id int) error
	HasSSHKey(ctx context.Context, key string) (bool, error)
	AddSSHKey(ctx context.Context, key string) error
}

// IdentityManager creates the deployment keypair and registers it.
type IdentityManager interface {
	PrepareIdentity(ctx context.Context, path string) error
}

// IdentityFunc adapts a plain function to IdentityManager.
type IdentityFunc func(ctx context.Context, path string) error

// PrepareIdentity implements IdentityManager.
func (run IdentityFunc) PrepareIdentity(ctx context.Context, path string) error {
	return run(ctx, path)
}

// HostKeyScanner reads the remote host keys on first contact.
type HostKeyScanner interface {
	Scan(ctx context.Context, host string, port int) (setup.HostKeys, error)
}

// TrustStore pins scanned host keys.
type TrustStore interface {
	Save(path string, contents []byte) error
}

// ServerLauncher starts the recipe server over SSH.
type ServerLauncher interface {
	Launch(ctx context.Context, ssh config.SSH, r recipe.Recipe) error
}

// FingerprintConfirmer gates first contact on manual approval. Nilly callers
// skip the gate; only the --verify-host-key flow sets one.
type FingerprintConfirmer func(ctx context.Context, fingerprints []string) (bool, error)

// Inputs selects what to rent and how far trust goes.
type Inputs struct {
	Recipe        recipe.Recipe
	Offer         vast.Offer
	CapUSD        float64
	Port          int
	DeploymentID  string
	VerifyHostKey bool
	ReadyTimeout  time.Duration
}

// Deps carries the provision seams. Fakes in tests, setup primitives live.
type Deps struct {
	Vast     VastAPI
	Identity IdentityManager
	Scanner  HostKeyScanner
	Trust    TrustStore
	Launcher ServerLauncher
	Clock    setup.Clock
	Confirm  FingerprintConfirmer
	Progress func(string)
}

// Provision rents the offer and prepares it to serve, persisting every
// transition. It returns the deployment in preparing state; the caller
// opens the tunnel, verifies the route, and supervises it.
func Provision(ctx context.Context, store *state.Store, dir string, inputs Inputs, deps Deps) (state.Deployment, error) {
	progress := deps.Progress
	if progress == nil {
		progress = func(string) {}
	}
	if inputs.Offer.PriceUnknown {
		return state.Deployment{}, fmt.Errorf("cannot rent offer %d with unknown price", inputs.Offer.ID)
	}
	now := deps.Clock.Now().UTC()
	deployment := state.Deployment{
		ID: inputs.DeploymentID, RecipeID: inputs.Recipe.ID, RecipeVersion: inputs.Recipe.Version,
		Pins: state.Pins{
			ImageDigest:     inputs.Recipe.Runtime.Image,
			ModelRepository: inputs.Recipe.Model.Repository,
			ModelRevision:   inputs.Recipe.Model.Revision,
			ModelFilename:   inputs.Recipe.Model.Filename,
			ModelSHA256:     inputs.Recipe.Model.SHA256,
		},
		Instance: state.Instance{Offer: state.OfferSnapshot{
			ID: inputs.Offer.ID, GPUName: inputs.Offer.GPUName, GPUCount: inputs.Offer.GPUCount,
			GPUVRAMGB: inputs.Offer.GPUVRAMGB, HourlyUSD: inputs.Offer.HourlyUSD, Location: inputs.Offer.Location,
		}},
		SSH: state.SSH{
			User:           "root",
			IdentityFile:   state.IdentityPath(dir, inputs.DeploymentID),
			KnownHostsFile: state.KnownHostsPath(dir, inputs.DeploymentID),
		},
		Route: state.Route{
			LocalHost: "127.0.0.1", LocalPort: inputs.Port,
			RemoteHost: "127.0.0.1", RemotePort: 30000,
		},
		Spend:     state.Spend{HourlyUSD: inputs.Offer.HourlyUSD},
		CapUSD:    inputs.CapUSD,
		State:     state.Planned,
		CreatedAt: now,
		UpdatedAt: now,
	}
	identityPath := state.IdentityPath(dir, inputs.DeploymentID)
	if err := deps.Identity.PrepareIdentity(ctx, identityPath); err != nil {
		return state.Deployment{}, err
	}
	deployment.State = state.Renting
	instanceID, err := deps.Vast.CreateInstance(ctx, inputs.Offer.ID, vast.CreateRequest{
		Image:  inputs.Recipe.Runtime.Image,
		DiskGB: inputs.Recipe.Requirements.MinimumDiskGB,
		Label:  "sovkit-" + inputs.DeploymentID,
	})
	if err != nil {
		return state.Deployment{}, err
	}
	deployment.Instance.ID = instanceID
	if err := store.Add(deployment); err != nil {
		return state.Deployment{}, destroyOrphan(ctx, store, dir, &deployment, deps.Vast, err)
	}
	store.SetActive(deployment.ID)
	if err := store.Save(dir); err != nil {
		return state.Deployment{}, destroyOrphan(ctx, store, dir, &deployment, deps.Vast, err)
	}
	progress(fmt.Sprintf("Instance %d rented, waiting until ready…", instanceID))
	instance, err := waitRunning(ctx, deps, deps.Vast, instanceID, inputs.ReadyTimeout)
	if err != nil {
		return state.Deployment{}, destroyOrphan(ctx, store, dir, &deployment, deps.Vast, err)
	}
	deployment.Instance.Status = instance.Status
	deployment.SSH.Host, deployment.SSH.Port = instance.SSHHost, instance.SSHPort
	if err := save(store, dir, &deployment); err != nil {
		return state.Deployment{}, destroyOrphan(ctx, store, dir, &deployment, deps.Vast, err)
	}
	progress("Instance ready, pinning host keys…")
	keys, err := deps.Scanner.Scan(ctx, instance.SSHHost, instance.SSHPort)
	if err != nil {
		return state.Deployment{}, destroyOrphan(ctx, store, dir, &deployment, deps.Vast, err)
	}
	for _, fingerprint := range keys.Fingerprints {
		progress("Remote host key: " + fingerprint)
	}
	if inputs.VerifyHostKey {
		if deps.Confirm == nil {
			return state.Deployment{}, destroyOrphan(ctx, store, dir, &deployment, deps.Vast, fmt.Errorf("host key verification needs a confirmer"))
		}
		confirmed, err := deps.Confirm(ctx, keys.Fingerprints)
		if err != nil {
			return state.Deployment{}, destroyOrphan(ctx, store, dir, &deployment, deps.Vast, err)
		}
		if !confirmed {
			return state.Deployment{}, destroyOrphan(ctx, store, dir, &deployment, deps.Vast, fmt.Errorf("host key not confirmed"))
		}
	}
	if err := deps.Trust.Save(state.KnownHostsPath(dir, inputs.DeploymentID), keys.Raw); err != nil {
		return state.Deployment{}, destroyOrphan(ctx, store, dir, &deployment, deps.Vast, err)
	}
	progress("Host keys pinned, launching server…")
	deployment.State = state.Preparing
	if err := save(store, dir, &deployment); err != nil {
		return state.Deployment{}, destroyOrphan(ctx, store, dir, &deployment, deps.Vast, err)
	}
	ssh := config.SSH{
		Host: instance.SSHHost, Port: instance.SSHPort, User: "root",
		IdentityFile: identityPath, KnownHostsFile: state.KnownHostsPath(dir, inputs.DeploymentID),
	}
	if err := deps.Launcher.Launch(ctx, ssh, inputs.Recipe); err != nil {
		return state.Deployment{}, destroyOrphan(ctx, store, dir, &deployment, deps.Vast, err)
	}
	return deployment, nil
}

func save(store *state.Store, dir string, deployment *state.Deployment) error {
	if err := store.Update(*deployment); err != nil {
		return err
	}
	return store.Save(dir)
}

// waitRunning polls until the instance runs. Exited instances fail fast:
// reviving a broken container needs an operator decision.
func waitRunning(ctx context.Context, deps Deps, vastAPI VastAPI, id int, timeout time.Duration) (vast.Instance, error) {
	deadline := deps.Clock.Now().Add(timeout)
	for {
		instance, err := vastAPI.GetInstance(ctx, id)
		if err != nil {
			return vast.Instance{}, err
		}
		if strings.EqualFold(instance.Status, "running") {
			return instance, nil
		}
		if strings.EqualFold(instance.Status, "exited") {
			return instance, fmt.Errorf("instance %d exited; destroy and re-provision it", id)
		}
		if !deps.Clock.Now().Before(deadline) {
			return instance, fmt.Errorf("instance %d not running after %s (status %q)", id, timeout, instance.Status)
		}
		if err := deps.Clock.Sleep(ctx, 5*time.Second); err != nil {
			return vast.Instance{}, err
		}
	}
}

// recordFailure marks the deployment failed and persists it, adding the
// record when an earlier persist never landed.
func recordFailure(store *state.Store, dir string, deployment *state.Deployment) error {
	deployment.State = state.Failed
	if _, ok := store.Get(deployment.ID); !ok {
		if err := store.Add(*deployment); err != nil {
			return err
		}
	}
	return save(store, dir, deployment)
}

// destroyOrphan stops billing for a rented-but-broken instance and records
// the failure. A failed destroy is reported loudly: the instance may bill.
func destroyOrphan(ctx context.Context, store *state.Store, dir string, deployment *state.Deployment, vastAPI VastAPI, cause error) error {
	derr := vastAPI.DestroyInstance(ctx, deployment.Instance.ID)
	serr := recordFailure(store, dir, deployment)
	switch {
	case derr != nil && serr != nil:
		return fmt.Errorf("%v; instance %d may still bill: destroy failed: %v (and the failure could not be recorded: %v)", cause, deployment.Instance.ID, derr, serr)
	case derr != nil:
		return fmt.Errorf("%v; instance %d may still bill: destroy failed: %v", cause, deployment.Instance.ID, derr)
	case serr != nil:
		return fmt.Errorf("%v; orphan instance %d destroyed but the failure was not recorded: %v", cause, deployment.Instance.ID, serr)
	default:
		return fmt.Errorf("%v; orphan instance %d destroyed", cause, deployment.Instance.ID)
	}
}
