package provision

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/state"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

type fakeVast struct {
	instanceID int
	statuses   []string
	calls      int
	createdReq vast.CreateRequest
	createErr  error
	destroyed  []int
	destroyErr error
}

func (fake *fakeVast) CreateInstance(_ context.Context, offerID int, request vast.CreateRequest) (int, error) {
	if fake.createErr != nil {
		return 0, fake.createErr
	}
	fake.createdReq = request
	_ = offerID
	return 123456, nil
}

func (fake *fakeVast) GetInstance(context.Context, int) (vast.Instance, error) {
	status := "running"
	if fake.calls < len(fake.statuses) {
		status = fake.statuses[fake.calls]
	}
	fake.calls++
	return vast.Instance{
		ID: 123456, Status: status,
		SSHHost: "h.example.test", SSHPort: 22022,
		Image: "ghcr.io/example/llama@sha256:abc",
	}, nil
}

func (fake *fakeVast) DestroyInstance(context.Context, int) error {
	fake.destroyed = append(fake.destroyed, 123456)
	return fake.destroyErr
}

func (fake *fakeVast) HasSSHKey(context.Context, string) (bool, error) {
	return false, nil
}

func (fake *fakeVast) AddSSHKey(context.Context, string) error {
	return nil
}

type fakeIdentity struct {
	prepared []string
	fail     error
}

func (fake *fakeIdentity) PrepareIdentity(_ context.Context, path string) error {
	if fake.fail != nil {
		return fake.fail
	}
	fake.prepared = append(fake.prepared, path)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte("PRIVATE"), 0o600); err != nil {
		return err
	}
	return os.WriteFile(path+".pub", []byte("ssh-ed25519 QUFBQQ== fake"), 0o600)
}

type fakeScanner struct {
	raw          string
	fingerprints []string
	fail         error
}

func (fake *fakeScanner) Scan(context.Context, string, int) (setup.HostKeys, error) {
	if fake.fail != nil {
		return setup.HostKeys{}, fake.fail
	}
	return setup.HostKeys{Raw: []byte(fake.raw), Fingerprints: fake.fingerprints}, nil
}

type fakeTrust struct {
	saved map[string]string
	fail  error
}

func (fake *fakeTrust) Save(path string, contents []byte) error {
	if fake.fail != nil {
		return fake.fail
	}
	if fake.saved == nil {
		fake.saved = map[string]string{}
	}
	fake.saved[path] = string(contents)
	return os.WriteFile(path, contents, 0o600)
}

type fakeLauncher struct {
	launched bool
	ssh      config.SSH
	recipeID string
	fail     error
}

func (fake *fakeLauncher) Launch(_ context.Context, ssh config.SSH, r recipe.Recipe) error {
	if fake.fail != nil {
		return fake.fail
	}
	fake.launched = true
	fake.ssh = ssh
	fake.recipeID = r.ID
	return nil
}

type fakeClock struct {
	now time.Time
}

func (clock *fakeClock) Now() time.Time { return clock.now }

func (clock *fakeClock) Sleep(ctx context.Context, duration time.Duration) error {
	clock.now = clock.now.Add(duration)
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func testRecipe() recipe.Recipe {
	return recipe.Recipe{
		Version: 1, ID: "qwen-solo-rtx5090", Name: "Qwen Solo", Kind: "gguf-text-generation",
		Runtime: recipe.Runtime{Engine: "llama-cpp", Image: "ghcr.io/example/llama@sha256:abc"},
		Model:   recipe.Model{Repository: "o/m", Revision: "319f741cce68d7914884900c138a1fbb70a42f30", Filename: "m.gguf"},
		Serve:   recipe.Serve{ContextWindow: 131072, MaxOutputTokens: 16384, MaxRunningRequests: 1},
		Requirements: recipe.Requirements{
			GPUModel: "RTX 5090", GPUCount: 1, StrictGPU: true, MinimumVRAMGB: 32, MinimumDiskGB: 100,
		},
	}
}

func testOffer() vast.Offer {
	return vast.Offer{
		ID: 7, GPUName: "RTX 5090", GPUCount: 1, GPUVRAMGB: 32.607,
		HourlyUSD: 0.42, Location: "FR",
	}
}

func testDeps(vastAPI *fakeVast) (*fakeIdentity, *fakeScanner, *fakeTrust, *fakeLauncher, *fakeClock, Deps) {
	identity := &fakeIdentity{}
	scanner := &fakeScanner{raw: "h ssh-ed25519 QUFBQQ==\n", fingerprints: []string{"SHA256:QUFBQQ== (ED25519)"}}
	trust := &fakeTrust{}
	launcher := &fakeLauncher{}
	clock := &fakeClock{now: time.Date(2026, 9, 7, 14, 32, 0, 0, time.UTC)}
	return identity, scanner, trust, launcher, clock, Deps{
		Vast:     vastAPI,
		Identity: identity,
		Scanner:  scanner,
		Trust:    trust,
		Launcher: launcher,
		Clock:    clock,
		Progress: func(string) {},
	}
}

func testInputs() Inputs {
	return Inputs{
		Recipe: testRecipe(), Offer: testOffer(),
		CapUSD: 5, Port: 30000, DeploymentID: "qwen-solo-20260907t1432",
		ReadyTimeout: time.Minute,
	}
}

func loadOne(t *testing.T, dir, id string) (state.Store, state.Deployment) {
	t.Helper()
	store, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	deployment, ok := store.Get(id)
	if !ok {
		t.Fatalf("deployment %q missing", id)
	}
	return store, deployment
}

func TestProvisionHappyPathPersistsTransitions(t *testing.T) {
	dir := state.Dir(t.TempDir())
	vastAPI := &fakeVast{}
	_, scanner, trust, launcher, _, deps := testDeps(vastAPI)
	var progressed []string
	deps.Progress = func(line string) { progressed = append(progressed, line) }

	deployment, err := Provision(context.Background(), &state.Store{Version: 1, Settings: state.DefaultSettings()}, dir, testInputs(), deps)
	if err != nil {
		t.Fatal(err)
	}
	if deployment.State != state.Preparing {
		t.Fatalf("state = %q", deployment.State)
	}
	if deployment.Instance.ID != 123456 || deployment.SSH.Host != "h.example.test" || deployment.SSH.Port != 22022 {
		t.Fatalf("instance not recorded: %+v", deployment.Instance)
	}
	if !launcher.launched || launcher.recipeID != "qwen-solo-rtx5090" {
		t.Fatalf("launcher = %+v", launcher)
	}
	if deployment.Spend.HourlyUSD != 0.42 || deployment.CapUSD != 5 {
		t.Fatalf("spend/cap = %+v", deployment.Spend)
	}
	saved, err := os.ReadFile(state.KnownHostsPath(dir, deployment.ID))
	if err != nil || string(saved) != scanner.raw {
		t.Fatalf("pinned = %q, %v", saved, err)
	}
	joined := strings.Join(progressed, "\n")
	if !strings.Contains(joined, "SHA256:QUFBQQ==") {
		t.Fatalf("fingerprints not displayed: %q", joined)
	}
	if trust.saved[state.KnownHostsPath(dir, deployment.ID)] != scanner.raw {
		t.Fatal("trust store not written")
	}
	_, persisted := loadOne(t, dir, deployment.ID)
	if persisted.State != state.Preparing || persisted.Instance.ID != 123456 {
		t.Fatalf("persisted = %+v", persisted)
	}
}

func TestProvisionRefusesUnknownPrice(t *testing.T) {
	dir := state.Dir(t.TempDir())
	vastAPI := &fakeVast{}
	_, _, _, _, _, deps := testDeps(vastAPI)
	inputs := testInputs()
	inputs.Offer.PriceUnknown = true
	store := &state.Store{Version: 1, Settings: state.DefaultSettings()}
	if _, err := Provision(context.Background(), store, dir, inputs, deps); err == nil {
		t.Fatal("expected unknown-price refusal")
	}
	if len(store.Deployments) != 0 {
		t.Fatal("must persist nothing on refusal")
	}
}

func TestProvisionRentFailurePersistsNothing(t *testing.T) {
	dir := state.Dir(t.TempDir())
	vastAPI := &fakeVast{createErr: errors.New("bid rejected")}
	_, _, _, _, _, deps := testDeps(vastAPI)
	store := &state.Store{Version: 1, Settings: state.DefaultSettings()}
	if _, err := Provision(context.Background(), store, dir, testInputs(), deps); err == nil {
		t.Fatal("expected rent error")
	}
	if len(vastAPI.destroyed) != 0 {
		t.Fatal("must not destroy what was never rented")
	}
	if len(store.Deployments) != 0 {
		t.Fatal("must persist nothing without an instance")
	}
}

func TestProvisionAddFailureStillDestroysRented(t *testing.T) {
	dir := state.Dir(t.TempDir())
	vastAPI := &fakeVast{}
	_, _, _, _, _, deps := testDeps(vastAPI)
	store := &state.Store{Version: 1, Settings: state.DefaultSettings()}
	inputs := testInputs()
	occupant := state.Deployment{
		ID: inputs.DeploymentID, RecipeID: "other", RecipeVersion: 1, State: state.Serving,
		SSH:   state.SSH{Host: "h", Port: 22, User: "root", IdentityFile: "i", KnownHostsFile: "k"},
		Route: state.Route{LocalHost: "127.0.0.1", LocalPort: 30001, RemoteHost: "127.0.0.1", RemotePort: 30000},
	}
	if err := store.Add(occupant); err != nil {
		t.Fatal(err)
	}
	_, err := Provision(context.Background(), store, dir, inputs, deps)
	if err == nil || !strings.Contains(err.Error(), "orphan instance 123456 destroyed") {
		t.Fatalf("expected orphan destroy, got %v", err)
	}
	if len(vastAPI.destroyed) != 1 {
		t.Fatalf("destroyed = %+v", vastAPI.destroyed)
	}
	persisted, ok := store.Get(inputs.DeploymentID)
	if !ok || persisted.State != state.Failed {
		t.Fatalf("record = %+v %v", persisted, ok)
	}
	if _, err := state.Load(dir); err != nil {
		t.Fatalf("failure not persisted: %v", err)
	}
}

func TestProvisionLaunchFailureDestroysOrphan(t *testing.T) {
	dir := state.Dir(t.TempDir())
	vastAPI := &fakeVast{}
	_, _, _, launcher, _, deps := testDeps(vastAPI)
	launcher.fail = errors.New("server exited")
	store := &state.Store{Version: 1, Settings: state.DefaultSettings()}
	_, err := Provision(context.Background(), store, dir, testInputs(), deps)
	if err == nil || !strings.Contains(err.Error(), "orphan instance 123456 destroyed") {
		t.Fatalf("expected orphan destroy, got %v", err)
	}
	if len(vastAPI.destroyed) != 1 {
		t.Fatalf("destroyed = %+v", vastAPI.destroyed)
	}
	_, persisted := loadOne(t, dir, testInputs().DeploymentID)
	if persisted.State != state.Failed {
		t.Fatalf("state = %q, want failed", persisted.State)
	}
}

func TestProvisionVerifyDeclinedDestroysOrphan(t *testing.T) {
	dir := state.Dir(t.TempDir())
	vastAPI := &fakeVast{}
	_, _, _, _, _, deps := testDeps(vastAPI)
	inputs := testInputs()
	inputs.VerifyHostKey = true
	deps.Confirm = func(context.Context, []string) (bool, error) { return false, nil }
	store := &state.Store{Version: 1, Settings: state.DefaultSettings()}
	_, err := Provision(context.Background(), store, dir, inputs, deps)
	if err == nil || !strings.Contains(err.Error(), "host key not confirmed") {
		t.Fatalf("expected trust refusal, got %v", err)
	}
	if len(vastAPI.destroyed) != 1 {
		t.Fatalf("destroyed = %+v", vastAPI.destroyed)
	}
	_, persisted := loadOne(t, dir, inputs.DeploymentID)
	if persisted.State != state.Failed {
		t.Fatalf("state = %q, want failed", persisted.State)
	}
}

func TestProvisionSkipsVerifyWhenFlagOff(t *testing.T) {
	dir := state.Dir(t.TempDir())
	vastAPI := &fakeVast{}
	_, _, _, _, _, deps := testDeps(vastAPI)
	deps.Confirm = func(context.Context, []string) (bool, error) {
		t.Fatal("confirm must not run without the verify flag")
		return false, nil
	}
	store := &state.Store{Version: 1, Settings: state.DefaultSettings()}
	if _, err := Provision(context.Background(), store, dir, testInputs(), deps); err != nil {
		t.Fatal(err)
	}
}

func TestProvisionTimeoutDestroysOrphan(t *testing.T) {
	dir := state.Dir(t.TempDir())
	vastAPI := &fakeVast{statuses: []string{"loading", "loading"}}
	_, _, _, _, _, deps := testDeps(vastAPI)
	inputs := testInputs()
	inputs.ReadyTimeout = time.Nanosecond
	store := &state.Store{Version: 1, Settings: state.DefaultSettings()}
	_, err := Provision(context.Background(), store, dir, inputs, deps)
	if err == nil || !strings.Contains(err.Error(), "may still bill") && len(vastAPI.destroyed) != 1 {
		t.Fatalf("expected orphan destroy, got %v %+v", err, vastAPI.destroyed)
	}
	_, persisted := loadOne(t, dir, inputs.DeploymentID)
	if persisted.State != state.Failed {
		t.Fatalf("state = %q, want failed", persisted.State)
	}
}
