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
	selected     vast.Offer
	confirmCost  bool
	confirmErr   error
	confirmKeys  bool
	confirmKeyErr error
	views        []OfferView
	fingerprints []string
	events       *[]string
}

func (f *fakeOperator) SelectOffer(_ context.Context, views []OfferView) (vast.Offer, error) {
	f.views = append([]OfferView(nil), views...)
	if f.selected.ID != 0 {
		return f.selected, nil
	}
	return views[0].Offer, nil
}

func (f *fakeOperator) ConfirmCost(_ context.Context, _ OfferView, _ int) (bool, error) {
	return f.confirmCost, f.confirmErr
}

func (f *fakeOperator) ConfirmHostKeys(_ context.Context, fingerprints []string) (bool, error) {
	f.fingerprints = append([]string(nil), fingerprints...)
	if f.events != nil {
		*f.events = append(*f.events, "confirm-host-keys")
	}
	return f.confirmKeys, f.confirmKeyErr
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
	calls int
	path  string
	raw   []byte
	events *[]string
	err   error
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
		Version: 1,
		ID:      "qwen-studio",
		Name:    "Qwen Studio",
		Kind:    "text-generation",
		Runtime: recipe.Runtime{Engine: "sglang", Image: "example/sglang@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		Model: recipe.Model{Repository: "Qwen/Qwen", Revision: strings.Repeat("a", 40)},
		Serve: recipe.Serve{ContextWindow: 128, MaxOutputTokens: 32, MaxRunningRequests: 1},
		Requirements: recipe.Requirements{MinimumVRAMGB: 96, MinimumDiskGB: 120},
	}
}

func testOptions() Options {
	return Options{ConfigPath: "/tmp/config.toml", IdentityFile: "/tmp/id", KnownHostsDir: "/tmp/known-hosts", OfferLimit: 10, PollInterval: time.Second, PollTimeout: 5 * time.Second}
}

func baseDependencies(api *fakeVastAPI, operator *fakeOperator, scanner *fakeScanner, trust *fakeTrustStore, launcher *fakeLauncher, clock *fakeClock, events *[]string) Dependencies {
	return Dependencies{
		NewAPI: func(string) VastAPI { return api },
		Operator: operator,
		HostKeyScanner: scanner,
		TrustStore: trust,
		ServerLauncher: launcher,
		Clock: clock,
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

func TestRunVastRejectsUnreadableIdentityBeforeSearching(t *testing.T) {
	api := &fakeVastAPI{}
	deps := baseDependencies(api, &fakeOperator{}, &fakeScanner{}, &fakeTrustStore{}, &fakeLauncher{}, &fakeClock{now: time.Unix(0, 0)}, nil)
	deps.ValidateIdentity = func(string) error { return errors.New("identity is unreadable") }
	_, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
	if err == nil || !strings.Contains(err.Error(), "identity is unreadable") {
		t.Fatalf("expected identity error, got %v", err)
	}
	if api.searchCalls != 0 {
		t.Fatalf("search calls = %d", api.searchCalls)
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
	if api.searchReq != (vast.SearchRequest{Limit: 10, MinimumVRAMGB: 96}) {
		t.Fatalf("search request = %#v", api.searchReq)
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
	want := []string{"create", "get:loading", "sleep", "get:running", "scan", "confirm-host-keys", "save-known-hosts", "launch-server", "save-config"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
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
	api := &fakeVastAPI{offers: []vast.Offer{{ID: 42, GPUVRAMGB: 96, HourlyUSD: 1}}, createID: 987, instances: []vast.Instance{{ID: 987, Status: "loading"}}}
	clock := &fakeClock{now: time.Unix(0, 0)}
	deps := baseDependencies(api, &fakeOperator{confirmCost: true}, &fakeScanner{}, &fakeTrustStore{}, &fakeLauncher{}, clock, nil)
	options := testOptions()
	options.PollTimeout = time.Second
	_, err := RunVast(context.Background(), "token", validRecipe(), options, deps)
	if err == nil || !strings.Contains(err.Error(), "instance 987") || !strings.Contains(err.Error(), "billing may still be active") {
		t.Fatalf("expected timeout warning, got %v", err)
	}
}

func TestRunVastDoesNothingTrustedWhenHostKeysAreDeclined(t *testing.T) {
	api := &fakeVastAPI{offers: []vast.Offer{{ID: 42, GPUVRAMGB: 96, HourlyUSD: 1}}, createID: 987, instances: []vast.Instance{{ID: 987, Status: "running", SSHHost: "gpu.example", SSHPort: 22022}}}
	scanner := &fakeScanner{keys: HostKeys{Raw: []byte("key"), Fingerprints: []string{"SHA256:abc"}}}
	trust := &fakeTrustStore{}
	launcher := &fakeLauncher{}
	deps := baseDependencies(api, &fakeOperator{confirmCost: true, confirmKeys: false}, scanner, trust, launcher, &fakeClock{now: time.Unix(0, 0)}, nil)
	_, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
	if err == nil {
		t.Fatal("expected cancellation")
	}
	if trust.calls != 0 || launcher.calls != 0 {
		t.Fatalf("trusted calls after decline: trust=%d launch=%d", trust.calls, launcher.calls)
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
	want := []string{"create", "get:running", "scan", "confirm-host-keys", "save-known-hosts", "launch-server", "save-config"}
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
