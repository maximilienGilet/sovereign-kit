package setup

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

type fakeVastAPI struct {
	offers      []vast.Offer
	instances   []vast.Instance
	searchReq   vast.SearchRequest
	searchCalls int
	createCalls int
	createID    int
	createOffer int
	createReq   vast.CreateRequest
	getCalls    int
	events      *[]string
}

func (f *fakeVastAPI) SearchOffers(_ context.Context, request vast.SearchRequest) ([]vast.Offer, error) {
	f.searchCalls++
	f.searchReq = request
	return f.offers, nil
}

func (f *fakeVastAPI) CreateInstance(_ context.Context, offerID int, request vast.CreateRequest) (int, error) {
	f.createCalls++
	f.createOffer = offerID
	f.createReq = request
	if f.events != nil {
		*f.events = append(*f.events, "create")
	}
	return f.createID, nil
}

func (f *fakeVastAPI) GetInstance(_ context.Context, _ int) (vast.Instance, error) {
	if f.getCalls >= len(f.instances) {
		return vast.Instance{}, errors.New("unexpected get")
	}
	instance := f.instances[f.getCalls]
	f.getCalls++
	if f.events != nil {
		*f.events = append(*f.events, "get:"+instance.Status)
	}
	return instance, nil
}

type fakeOperator struct {
	selected      vast.Offer
	confirmCost   bool
	confirmErr    error
	confirmKeys   bool
	confirmKeyErr error
	views         []OfferView
	fingerprints  []string
	events        *[]string
	selectCalls   int
	confirmedView OfferView
	recovery      InstanceRecovery
}

func (f *fakeOperator) SelectOffer(_ context.Context, views []OfferView) (vast.Offer, error) {
	f.selectCalls++
	f.views = append([]OfferView(nil), views...)
	if f.selected.ID != 0 {
		return f.selected, nil
	}
	return views[0].Offer, nil
}

func (f *fakeOperator) ConfirmCost(_ context.Context, view OfferView, _ int) (bool, error) {
	f.confirmedView = view
	return f.confirmCost, f.confirmErr
}

func (f *fakeOperator) ConfirmHostKeys(_ context.Context, fingerprints []string) (bool, error) {
	f.fingerprints = append([]string(nil), fingerprints...)
	if f.events != nil {
		*f.events = append(*f.events, "confirm-host-keys")
	}
	return f.confirmKeys, f.confirmKeyErr
}

func (f *fakeOperator) InstanceCreated(recovery InstanceRecovery) {
	f.recovery = recovery
}

type fakeScanner struct {
	keys      HostKeys
	calls     int
	host      string
	port      int
	events    *[]string
	scanError error
}

func (f *fakeScanner) Scan(_ context.Context, host string, port int) (HostKeys, error) {
	f.calls++
	f.host = host
	f.port = port
	if f.events != nil {
		*f.events = append(*f.events, "scan")
	}
	return f.keys, f.scanError
}

type fakeTrustStore struct {
	calls  int
	path   string
	raw    []byte
	events *[]string
	err    error
}

func (f *fakeTrustStore) Save(path string, raw []byte) error {
	f.calls++
	f.path = path
	f.raw = append([]byte(nil), raw...)
	if f.events != nil {
		*f.events = append(*f.events, "save-known-hosts")
	}
	return f.err
}

type fakeLauncher struct {
	calls  int
	ssh    config.SSH
	recipe recipe.Recipe
	events *[]string
	err    error
}

func (f *fakeLauncher) Launch(_ context.Context, ssh config.SSH, r recipe.Recipe) error {
	f.calls++
	f.ssh = ssh
	f.recipe = r
	if f.events != nil {
		*f.events = append(*f.events, "launch-server")
	}
	return f.err
}

type fakeClock struct {
	now    time.Time
	sleeps []time.Duration
	events *[]string
}

func (f *fakeClock) Now() time.Time { return f.now }

func (f *fakeClock) Sleep(ctx context.Context, duration time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.sleeps = append(f.sleeps, duration)
	f.now = f.now.Add(duration)
	if f.events != nil {
		*f.events = append(*f.events, "sleep")
	}
	return nil
}

func validRecipe() recipe.Recipe {
	return recipe.Recipe{
		Version:      1,
		ID:           "qwen-studio",
		Name:         "Qwen Studio",
		Kind:         "text-generation",
		Runtime:      recipe.Runtime{Engine: "sglang", Image: "example/sglang@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		Model:        recipe.Model{Repository: "Qwen/Qwen", Revision: strings.Repeat("a", 40)},
		Serve:        recipe.Serve{ContextWindow: 128, MaxOutputTokens: 32, MaxRunningRequests: 1},
		Requirements: recipe.Requirements{MinimumVRAMGB: 96, MinimumDiskGB: 120},
	}
}

func strictRecipe() recipe.Recipe {
	r := validRecipe()
	r.Profile = recipe.Profile{Status: "experimental", Summary: "strict test", Evidence: "strict test evidence"}
	r.Requirements = recipe.Requirements{
		GPUModel:      "RTX 5090",
		GPUCount:      1,
		StrictGPU:     true,
		MinimumVRAMGB: 32,
		MinimumDiskGB: 100,
	}
	return r
}

func testOptions() Options {
	return Options{ConfigPath: "/tmp/config.toml", IdentityFile: "/tmp/id", KnownHostsDir: "/tmp/known-hosts", OfferLimit: 10, PollInterval: time.Second, PollTimeout: 5 * time.Second}
}

func baseDependencies(api *fakeVastAPI, operator *fakeOperator, scanner *fakeScanner, trust *fakeTrustStore, launcher *fakeLauncher, clock *fakeClock, events *[]string) Dependencies {
	return Dependencies{
		NewAPI:         func(string) VastAPI { return api },
		Operator:       operator,
		HostKeyScanner: scanner,
		TrustStore:     trust,
		ServerLauncher: launcher,
		Clock:          clock,
		SaveConfig: func(_ string, _ config.Config) error {
			if events != nil {
				*events = append(*events, "save-config")
			}
			return nil
		},
		ValidateIdentity: func(string) error { return nil },
	}
}

func TestRunVastRejectsMissingAPIKey(t *testing.T) {
	deps := Dependencies{NewAPI: func(string) VastAPI { t.Fatal("API constructed"); return nil }}
	_, err := RunVast(context.Background(), " \t", validRecipe(), testOptions(), deps)
	if err == nil || !strings.Contains(err.Error(), "API key") {
		t.Fatalf("expected missing API key error, got %v", err)
	}
}

func TestRunVastRejectsUnreadableIdentityAfterCostBeforeCreation(t *testing.T) {
	api := &fakeVastAPI{offers: []vast.Offer{{ID: 42, GPUVRAMGB: 96, HourlyUSD: 1}}}
	deps := baseDependencies(api, &fakeOperator{confirmCost: true}, &fakeScanner{}, &fakeTrustStore{}, &fakeLauncher{}, &fakeClock{now: time.Unix(0, 0)}, nil)
	deps.ValidateIdentity = func(string) error { return errors.New("identity is unreadable") }
	_, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
	if err == nil || !strings.Contains(err.Error(), "identity is unreadable") {
		t.Fatalf("expected identity error, got %v", err)
	}
	if api.searchCalls != 1 || api.createCalls != 0 {
		t.Fatalf("search calls=%d create calls=%d", api.searchCalls, api.createCalls)
	}
}

func TestRunVastRejectsNoEligibleOffers(t *testing.T) {
	api := &fakeVastAPI{offers: []vast.Offer{{ID: 24, GPUVRAMGB: 24, HourlyUSD: 1}}}
	operator := &fakeOperator{}
	deps := baseDependencies(api, operator, &fakeScanner{}, &fakeTrustStore{}, &fakeLauncher{}, &fakeClock{now: time.Unix(0, 0)}, nil)
	_, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
	if err == nil || !strings.Contains(err.Error(), "eligible") {
		t.Fatalf("expected no eligible offers error, got %v", err)
	}
	if len(operator.views) != 0 {
		t.Fatalf("operator received %d views", len(operator.views))
	}
}

func TestRunVastPassesStrictRequirementsAndRejectsMismatchedHardware(t *testing.T) {
	api := &fakeVastAPI{offers: []vast.Offer{
		{ID: 1, GPUName: "RTX 4090", GPUCount: 1, GPUVRAMGB: 32, DiskSpaceGB: 100, HourlyUSD: 0.25},
		{ID: 2, GPUName: "RTX 5090", GPUCount: 2, GPUVRAMGB: 32, DiskSpaceGB: 100, HourlyUSD: 0.5},
		{ID: 4, GPUName: "RTX 5090", GPUCount: 1, GPUVRAMGB: 32, DiskSpaceGB: 50, HourlyUSD: 0.6},
		{ID: 3, GPUName: "RTX 5090", GPUCount: 1, GPUVRAMGB: 32, DiskSpaceGB: 100, HourlyUSD: 0.75},
	}}
	operator := &fakeOperator{confirmCost: false}
	deps := baseDependencies(api, operator, &fakeScanner{}, &fakeTrustStore{}, &fakeLauncher{}, &fakeClock{now: time.Unix(0, 0)}, nil)
	_, err := RunVast(context.Background(), "token", strictRecipe(), testOptions(), deps)
	if err == nil || !strings.Contains(err.Error(), "cancel") {
		t.Fatalf("error=%v", err)
	}
	wantRequest := vast.SearchRequest{Limit: 10, GPUModel: "RTX 5090", GPUCount: 1, StrictGPU: true, MinimumVRAMGB: 32, MinimumDiskGB: 100}
	if !reflect.DeepEqual(api.searchReq, wantRequest) {
		t.Fatalf("search request=%#v want=%#v", api.searchReq, wantRequest)
	}
	if len(operator.views) != 1 || operator.views[0].Offer.ID != 3 || operator.views[0].Recipe.ID != strictRecipe().ID {
		t.Fatalf("views=%#v", operator.views)
	}
}

func TestRunVastSortsOffersAndBuildsCostScenarios(t *testing.T) {
	offers := []vast.Offer{
		{ID: 9, GPUVRAMGB: 96, HourlyUSD: 1.25},
		{ID: 3, GPUVRAMGB: 96, HourlyUSD: 0.5},
		{ID: 7, GPUVRAMGB: 96, HourlyUSD: 1.25},
		{ID: 24, GPUVRAMGB: 24, HourlyUSD: 0.1},
	}
	api := &fakeVastAPI{offers: offers}
	operator := &fakeOperator{confirmCost: false}
	deps := baseDependencies(api, operator, &fakeScanner{}, &fakeTrustStore{}, &fakeLauncher{}, &fakeClock{now: time.Unix(0, 0)}, nil)
	_, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
	if err == nil {
		t.Fatal("expected cancellation")
	}
	if got := []int{operator.views[0].Offer.ID, operator.views[1].Offer.ID, operator.views[2].Offer.ID}; !reflect.DeepEqual(got, []int{3, 7, 9}) {
		t.Fatalf("offer order = %v", got)
	}
	if operator.views[1].MonthlyUSD != 912.5 || operator.views[1].AnnualUSD != 10950 {
		t.Fatalf("cost scenarios = %#v", operator.views[1])
	}
	if !reflect.DeepEqual(api.searchReq, vast.SearchRequest{Limit: 10, MinimumVRAMGB: 96, MinimumDiskGB: 120}) {
		t.Fatalf("search request = %#v", api.searchReq)
	}
}

func TestRunVastLegacyAutoSelectStillRequiresExplicitOfferSelection(t *testing.T) {
	api := &fakeVastAPI{offers: []vast.Offer{
		{ID: 9, GPUVRAMGB: 96, HourlyUSD: 1.25},
		{ID: 3, GPUVRAMGB: 96, HourlyUSD: 0.5},
	}}
	operator := &fakeOperator{selected: vast.Offer{ID: 9}, confirmCost: false}
	deps := baseDependencies(api, operator, &fakeScanner{}, &fakeTrustStore{}, &fakeLauncher{}, &fakeClock{now: time.Unix(0, 0)}, nil)
	options := testOptions()
	options.AutoSelectOffer = true

	_, err := RunVast(context.Background(), "token", validRecipe(), options, deps)
	if err == nil || !strings.Contains(err.Error(), "cancel") {
		t.Fatalf("error=%v", err)
	}
	if operator.selectCalls != 1 || operator.confirmedView.Offer.ID != 9 || operator.confirmedView.Recipe.ID != validRecipe().ID {
		t.Fatalf("select calls=%d confirmed=%#v", operator.selectCalls, operator.confirmedView)
	}
}

func TestRunVastKeepsDetailedOperatorOfferSelectionForCustomModel(t *testing.T) {
	api := &fakeVastAPI{offers: []vast.Offer{
		{ID: 3, GPUName: "A100", GPUVRAMGB: 96, HourlyUSD: 0.5},
		{ID: 9, GPUName: "H100", GPUVRAMGB: 96, HourlyUSD: 1.25},
	}}
	operator := &fakeOperator{selected: vast.Offer{ID: 9}, confirmCost: false}
	deps := baseDependencies(api, operator, &fakeScanner{}, &fakeTrustStore{}, &fakeLauncher{}, &fakeClock{now: time.Unix(0, 0)}, nil)

	_, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
	if err == nil || !strings.Contains(err.Error(), "cancel") {
		t.Fatalf("error=%v", err)
	}
	if operator.selectCalls != 1 || len(operator.views) != 2 || operator.confirmedView.Offer.ID != 9 {
		t.Fatalf("select calls=%d views=%#v confirmed=%d", operator.selectCalls, operator.views, operator.confirmedView.Offer.ID)
	}
}

func TestRunVastDoesNotCreateWhenCostIsDeclined(t *testing.T) {
	events := []string{}
	api := &fakeVastAPI{offers: []vast.Offer{{ID: 42, GPUVRAMGB: 96, HourlyUSD: 1}}}
	operator := &fakeOperator{confirmCost: false}
	scanner := &fakeScanner{}
	trust := &fakeTrustStore{}
	launcher := &fakeLauncher{}
	deps := baseDependencies(api, operator, scanner, trust, launcher, &fakeClock{now: time.Unix(0, 0)}, &events)
	_, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
	if err == nil || !strings.Contains(err.Error(), "cancel") {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if api.createCalls != 0 || scanner.calls != 0 || launcher.calls != 0 || trust.calls != 0 {
		t.Fatalf("post-decline calls: create=%d scan=%d launch=%d save=%d", api.createCalls, scanner.calls, launcher.calls, trust.calls)
	}
}

func TestRunVastPreparesIdentityAfterCostAndBeforeCreate(t *testing.T) {
	api, operator, _, _, _, _, deps := successfulSetup()
	events := []string{}
	api.events = &events
	operator.confirmedView = OfferView{}
	deps.PrepareIdentity = func(context.Context) error {
		events = append(events, "prepare-identity")
		return nil
	}
	deps.ValidateIdentity = func(string) error {
		events = append(events, "validate-identity")
		return nil
	}

	if _, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps); err != nil {
		t.Fatal(err)
	}
	prepareIndex, validateIndex, createIndex := -1, -1, -1
	for index, event := range events {
		switch event {
		case "prepare-identity":
			prepareIndex = index
		case "validate-identity":
			validateIndex = index
		case "create":
			createIndex = index
		}
	}
	if prepareIndex < 0 || validateIndex <= prepareIndex || createIndex <= validateIndex {
		t.Fatalf("events=%v", events)
	}
}

func TestRunVastDoesNotPrepareIdentityWhenCostIsDeclined(t *testing.T) {
	api := &fakeVastAPI{offers: []vast.Offer{{ID: 42, GPUVRAMGB: 96, HourlyUSD: 1}}}
	operator := &fakeOperator{confirmCost: false}
	deps := baseDependencies(api, operator, &fakeScanner{}, &fakeTrustStore{}, &fakeLauncher{}, &fakeClock{now: time.Unix(0, 0)}, nil)
	prepareCalls := 0
	deps.PrepareIdentity = func(context.Context) error {
		prepareCalls++
		return nil
	}

	_, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
	if err == nil || prepareCalls != 0 {
		t.Fatalf("error=%v prepare calls=%d", err, prepareCalls)
	}
}

func successfulSetup() (*fakeVastAPI, *fakeOperator, *fakeScanner, *fakeTrustStore, *fakeLauncher, *fakeClock, Dependencies) {
	events := []string{}
	api := &fakeVastAPI{offers: []vast.Offer{{ID: 42, GPUVRAMGB: 96, HourlyUSD: 1}}, createID: 987, instances: []vast.Instance{{ID: 987, Status: "loading"}, {ID: 987, Status: "running", SSHHost: "gpu.example", SSHPort: 22022}}, events: &events}
	operator := &fakeOperator{confirmCost: true, confirmKeys: true, events: &events}
	scanner := &fakeScanner{keys: HostKeys{Raw: []byte("gpu.example ssh-ed25519 AAAA\n"), Fingerprints: []string{"SHA256:abc"}}, events: &events}
	trust := &fakeTrustStore{events: &events}
	launcher := &fakeLauncher{events: &events}
	clock := &fakeClock{now: time.Unix(0, 0), events: &events}
	deps := baseDependencies(api, operator, scanner, trust, launcher, clock, &events)
	return api, operator, scanner, trust, launcher, clock, deps
}

func TestRunVastPollsUntilRunningSSHDetailsExist(t *testing.T) {
	api, _, _, _, _, _, deps := successfulSetup()
	events := []string{}
	api.events = &events
	deps.Operator.(*fakeOperator).events = &events
	deps.HostKeyScanner.(*fakeScanner).events = &events
	deps.TrustStore.(*fakeTrustStore).events = &events
	deps.ServerLauncher.(*fakeLauncher).events = &events
	deps.Clock.(*fakeClock).events = &events
	deps.SaveConfig = func(string, config.Config) error { events = append(events, "save-config"); return nil }
	result, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if result.InstanceID != 987 {
		t.Fatalf("instance ID = %d", result.InstanceID)
	}
	want := []string{"create", "get:loading", "sleep", "get:running", "scan", "save-known-hosts", "launch-server", "save-config"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

func TestRunVastPublishesExactRecoveryAfterCreationEvenWhenWaitingFails(t *testing.T) {
	api := &recoveryVastAPI{fakeVastAPI: fakeVastAPI{
		offers:    []vast.Offer{{ID: 42, GPUVRAMGB: 96, HourlyUSD: 1}},
		createID:  987,
		instances: []vast.Instance{{ID: 987, Status: "offline"}},
	}}
	operator := &fakeOperator{confirmCost: true}
	deps := baseDependencies(&api.fakeVastAPI, operator, &fakeScanner{}, &fakeTrustStore{}, &fakeLauncher{}, &fakeClock{now: time.Unix(0, 0)}, nil)
	deps.NewAPI = func(string) VastAPI { return api }

	_, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
	if err == nil {
		t.Fatal("expected waiting failure")
	}
	if operator.recovery.InstanceID != 987 || operator.recovery.Destroy == nil {
		t.Fatalf("recovery=%#v", operator.recovery)
	}
}

type recoveryVastAPI struct {
	fakeVastAPI
}

func (f *recoveryVastAPI) DestroyInstance(context.Context, int) error { return nil }

func (f *recoveryVastAPI) InstanceExists(context.Context, int) (bool, error) { return false, nil }

func TestWaitForRunningAllowsBlankStatusAsTransient(t *testing.T) {
	api := &fakeVastAPI{instances: []vast.Instance{{ID: 987, Status: " \t"}, {ID: 987, Status: "loading"}, {ID: 987, Status: "running", SSHHost: "gpu.example", SSHPort: 22}}}
	instance, err := waitForRunning(context.Background(), api, 987, testOptions(), &fakeClock{now: time.Unix(0, 0)}, nil)
	if err != nil || instance.ID != 987 {
		t.Fatalf("instance=%#v err=%v", instance, err)
	}
}

func TestWaitForRunningAllowsCreatedStatusAsTransient(t *testing.T) {
	api := &fakeVastAPI{instances: []vast.Instance{{ID: 987, Status: "created"}, {ID: 987, Status: "loading"}, {ID: 987, Status: "running", SSHHost: "gpu.example", SSHPort: 22}}}
	instance, err := waitForRunning(context.Background(), api, 987, testOptions(), &fakeClock{now: time.Unix(0, 0)}, nil)
	if err != nil || instance.ID != 987 {
		t.Fatalf("instance=%#v err=%v", instance, err)
	}
}

func TestWaitForRunningTimesOutOnPersistentlyBlankStatus(t *testing.T) {
	api := &fakeVastAPI{instances: []vast.Instance{{ID: 987, Status: ""}, {ID: 987, Status: ""}}}
	options := testOptions()
	options.PollTimeout = time.Second
	_, err := waitForRunning(context.Background(), api, 987, options, &fakeClock{now: time.Unix(0, 0)}, nil)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error=%v", err)
	}
}

func TestWaitForRunningReturnsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := waitForRunning(ctx, &fakeVastAPI{}, 987, testOptions(), &fakeClock{now: time.Unix(0, 0)}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
}

func TestRunVastStopsOnTerminalInstanceStatus(t *testing.T) {
	for _, status := range []string{"exited", "unknown", "offline"} {
		t.Run(status, func(t *testing.T) {
			api := &fakeVastAPI{offers: []vast.Offer{{ID: 42, GPUVRAMGB: 96, HourlyUSD: 1}}, createID: 987, instances: []vast.Instance{{ID: 987, Status: status}}}
			deps := baseDependencies(api, &fakeOperator{confirmCost: true}, &fakeScanner{}, &fakeTrustStore{}, &fakeLauncher{}, &fakeClock{now: time.Unix(0, 0)}, nil)
			_, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
			if err == nil || !strings.Contains(err.Error(), "instance 987") || !strings.Contains(err.Error(), "billing may still be active") {
				t.Fatalf("expected paid-instance error, got %v", err)
			}
		})
	}
}

func TestRunVastTimesOutWithInstanceAndBillingWarning(t *testing.T) {
	api := &fakeVastAPI{offers: []vast.Offer{{ID: 42, GPUVRAMGB: 96, HourlyUSD: 1}}, createID: 987, instances: []vast.Instance{{ID: 987, Status: "loading"}, {ID: 987, Status: "loading"}}}
	clock := &fakeClock{now: time.Unix(0, 0)}
	deps := baseDependencies(api, &fakeOperator{confirmCost: true}, &fakeScanner{}, &fakeTrustStore{}, &fakeLauncher{}, clock, nil)
	options := testOptions()
	options.PollTimeout = time.Second
	_, err := RunVast(context.Background(), "token", validRecipe(), options, deps)
	if err == nil || !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "instance 987") || !strings.Contains(err.Error(), "billing may still be active") {
		t.Fatalf("expected timeout warning, got %v", err)
	}
}

func TestRunVastPinsFirstCompleteHostKeySetWithoutConfirmation(t *testing.T) {
	api := &fakeVastAPI{offers: []vast.Offer{{ID: 42, GPUVRAMGB: 96, HourlyUSD: 1}}, createID: 987, instances: []vast.Instance{{ID: 987, Status: "running", SSHHost: "gpu.example", SSHPort: 22022}}}
	scanner := &fakeScanner{keys: HostKeys{Raw: []byte("key"), Fingerprints: []string{"SHA256:abc"}}}
	trust := &fakeTrustStore{}
	launcher := &fakeLauncher{}
	deps := baseDependencies(api, &fakeOperator{confirmCost: true, confirmKeys: false}, scanner, trust, launcher, &fakeClock{now: time.Unix(0, 0)}, nil)
	_, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
	if err != nil {
		t.Fatalf("first-use pinning failed: %v", err)
	}
	if trust.calls != 1 || launcher.calls != 1 {
		t.Fatalf("first-use trust not persisted before launch: trust=%d launch=%d", trust.calls, launcher.calls)
	}
}
func TestRunVastDoesNotLaunchWhenTrustPersistenceFails(t *testing.T) {
	api := &fakeVastAPI{offers: []vast.Offer{{ID: 42, GPUVRAMGB: 96, HourlyUSD: 1}}, createID: 987, instances: []vast.Instance{{ID: 987, Status: "running", SSHHost: "gpu.example", SSHPort: 22022}}}
	trust := &fakeTrustStore{err: errors.New("trust persistence failed")}
	launcher := &fakeLauncher{}
	deps := baseDependencies(api, &fakeOperator{confirmCost: true, confirmKeys: true}, &fakeScanner{keys: HostKeys{Raw: []byte("key"), Fingerprints: []string{"SHA256:abc"}}}, trust, launcher, &fakeClock{now: time.Unix(0, 0)}, nil)
	_, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
	if err == nil || !strings.Contains(err.Error(), "instance 987") || !strings.Contains(err.Error(), "billing may still be active") {
		t.Fatalf("expected trust persistence warning, got %v", err)
	}
	if launcher.calls != 0 {
		t.Fatalf("launcher calls = %d", launcher.calls)
	}
}

func TestRunVastLaunchesOnlyAfterTrustPersistence(t *testing.T) {
	events := []string{}
	api := &fakeVastAPI{offers: []vast.Offer{{ID: 42, GPUVRAMGB: 96, HourlyUSD: 1}}, createID: 987, instances: []vast.Instance{{ID: 987, Status: "running", SSHHost: "gpu.example", SSHPort: 22022}}, events: &events}
	operator := &fakeOperator{confirmCost: true, confirmKeys: true, events: &events}
	scanner := &fakeScanner{keys: HostKeys{Raw: []byte("key"), Fingerprints: []string{"SHA256:abc"}}, events: &events}
	trust := &fakeTrustStore{events: &events}
	launcher := &fakeLauncher{events: &events}
	deps := baseDependencies(api, operator, scanner, trust, launcher, &fakeClock{now: time.Unix(0, 0), events: &events}, &events)
	deps.SaveConfig = func(string, config.Config) error { events = append(events, "save-config"); return nil }
	if _, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	want := []string{"create", "get:running", "scan", "save-known-hosts", "launch-server", "save-config"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

func TestRunVastSavesVastConfigurationAfterLaunch(t *testing.T) {
	api := &fakeVastAPI{offers: []vast.Offer{{ID: 42, GPUVRAMGB: 96, HourlyUSD: 1}}, createID: 987, instances: []vast.Instance{{ID: 987, Status: "running", SSHHost: "gpu.example", SSHPort: 22022}}}
	launcher := &fakeLauncher{}
	trust := &fakeTrustStore{}
	var saved config.Config
	deps := baseDependencies(api, &fakeOperator{confirmCost: true, confirmKeys: true}, &fakeScanner{keys: HostKeys{Raw: []byte("key"), Fingerprints: []string{"SHA256:abc"}}}, trust, launcher, &fakeClock{now: time.Unix(0, 0)}, nil)
	deps.SaveConfig = func(path string, cfg config.Config) error {
		if launcher.calls != 1 || trust.calls != 1 {
			t.Fatalf("config saved before trusted launch: trust=%d launch=%d", trust.calls, launcher.calls)
		}
		if path != "/tmp/config.toml" {
			t.Fatalf("config path = %q", path)
		}
		saved = cfg
		return nil
	}
	result, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if result.ConfigPath != "/tmp/config.toml" || saved.Provider.Kind != "vast" || saved.Provider.InstanceID != 987 {
		t.Fatalf("result/config = %#v / %#v", result, saved)
	}
	knownHostsPath := filepath.Join("/tmp/known-hosts", "vast-987_known_hosts")
	if saved.SSH.KnownHostsFile != knownHostsPath || saved.SSH.Host != "gpu.example" || saved.SSH.Port != 22022 {
		t.Fatalf("saved SSH = %#v", saved.SSH)
	}
}

func TestRunVastRejectsIncompleteHostKeys(t *testing.T) {
	for name, keys := range map[string]HostKeys{
		"missing raw":          {Fingerprints: []string{"SHA256:abc"}},
		"missing fingerprints": {Raw: []byte("key")},
	} {
		t.Run(name, func(t *testing.T) {
			api := &fakeVastAPI{offers: []vast.Offer{{ID: 42, GPUVRAMGB: 96, HourlyUSD: 1}}, createID: 987, instances: []vast.Instance{{ID: 987, Status: "running", SSHHost: "gpu.example", SSHPort: 22022}}}
			scanner := &fakeScanner{keys: keys}
			operator := &fakeOperator{confirmCost: true, confirmKeys: true}
			trust := &fakeTrustStore{}
			deps := baseDependencies(api, operator, scanner, trust, &fakeLauncher{}, &fakeClock{now: time.Unix(0, 0)}, nil)
			_, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
			if err == nil || !strings.Contains(err.Error(), "instance 987") || trust.calls != 0 {
				t.Fatalf("expected incomplete host keys paid error, got %v", err)
			}
		})
	}
}
