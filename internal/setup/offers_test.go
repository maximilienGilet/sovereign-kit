package setup

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

func TestSearchEligibleOffersRetainsLoadedCountAndStrictHardware(t *testing.T) {
	api := &fakeVastAPI{offers: []vast.Offer{
		{ID: 1, GPUName: "RTX 5090", GPUCount: 1, GPUVRAMGB: 32, DiskSpaceGB: 100, HourlyUSD: 1, Location: "FR"},
		{ID: 2, GPUName: "RTX 4090", GPUCount: 1, GPUVRAMGB: 32, DiskSpaceGB: 100, HourlyUSD: 0.1, Location: "FR"},
		{ID: 3, GPUName: "RTX 5090", GPUCount: 2, GPUVRAMGB: 32, DiskSpaceGB: 100, HourlyUSD: 0.1, Location: "FR"},
		{ID: 4, GPUName: "RTX 5090", GPUCount: 1, GPUVRAMGB: 32, DiskSpaceGB: 100, HourlyUSD: 0.1, Location: "US"},
	}}
	result, err := SearchEligibleOffers(context.Background(), api, strictRecipe(), OfferQuery{Countries: []string{"fr"}}, 100)
	if err != nil || result.Loaded != 4 || result.Limit != 100 || len(result.Views) != 1 || result.Views[0].Offer.ID != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if !reflect.DeepEqual(api.searchReq.Countries, []string{"FR"}) || api.searchReq.MinimumVRAMGB != 32 || !api.searchReq.StrictGPU {
		t.Fatalf("unsafe query: %+v", api.searchReq)
	}
	result, err = SearchEligibleOffers(context.Background(), api, strictRecipe(), OfferQuery{Countries: []string{"DE"}}, 100)
	if err != nil || len(result.Views) != 0 || result.Loaded != 4 {
		t.Fatalf("empty filtered result=%+v err=%v", result, err)
	}
	r := strictRecipe()
	r.Requirements.StrictGPU = false
	result, err = SearchEligibleOffers(context.Background(), api, r, OfferQuery{}, 0)
	if err != nil || len(result.Views) != 4 || result.Limit != 5 {
		t.Fatalf("preferred hardware became strict: %+v %v", result, err)
	}
}

func TestSearchEligibleOffersSortsKnownMetricsAndKeepsUnknownInspectable(t *testing.T) {
	api := &fakeVastAPI{offers: []vast.Offer{
		{ID: 9, GPUVRAMGB: 96, HourlyUSD: 1, Reliability: 0.9},
		{ID: 2, GPUVRAMGB: 96, HourlyUSD: 1, Reliability: 0.9},
		{ID: 3, GPUVRAMGB: 96, HourlyUSD: 0.5, Reliability: 0.8},
		{ID: 4, GPUVRAMGB: 96, PriceUnknown: true, Reliability: 0.9},
		{ID: 5, GPUVRAMGB: 96, HourlyUSD: 0, ReliabilityUnknown: true},
	}}
	for _, tc := range []struct {
		sort vast.OfferSort
		ids  []int
	}{{vast.SortPrice, []int{5, 3, 2, 9, 4}}, {vast.SortReliability, []int{2, 9, 4, 3, 5}}} {
		result, err := SearchEligibleOffers(context.Background(), api, validRecipe(), OfferQuery{Sort: tc.sort}, 100)
		if err != nil {
			t.Fatal(err)
		}
		var ids []int
		for _, view := range result.Views {
			ids = append(ids, view.Offer.ID)
		}
		if !reflect.DeepEqual(ids, tc.ids) {
			t.Fatalf("sort %s ids=%v want %v", tc.sort, ids, tc.ids)
		}
	}
}

type browserOperator struct {
	fakeOperator
	browse func(context.Context, recipe.Recipe, OfferSearch) (vast.Offer, error)
}

func (b *browserOperator) BrowseOffers(ctx context.Context, r recipe.Recipe, search OfferSearch) (vast.Offer, error) {
	return b.browse(ctx, r, search)
}

type searchAPI struct {
	fakeVastAPI
	search func(context.Context, vast.SearchRequest) ([]vast.Offer, error)
}

func (a *searchAPI) SearchOffers(ctx context.Context, q vast.SearchRequest) ([]vast.Offer, error) {
	return a.search(ctx, q)
}

func TestRunVastBrowserSelectionUsesCurrentAuthoritativeFacts(t *testing.T) {
	api := &fakeVastAPI{offers: []vast.Offer{{ID: 42, GPUVRAMGB: 96, HourlyUSD: 2}}}
	operator := &browserOperator{fakeOperator: fakeOperator{confirmCost: false}}
	operator.browse = func(ctx context.Context, r recipe.Recipe, search OfferSearch) (vast.Offer, error) {
		if r.ID != validRecipe().ID {
			t.Fatal("recipe metadata missing")
		}
		result, err := search(ctx, OfferQuery{})
		if err != nil {
			return vast.Offer{}, err
		}
		result.Views[0].Offer.HourlyUSD = 0.001
		return result.Views[0].Offer, nil
	}
	deps := baseDependencies(api, &operator.fakeOperator, &fakeScanner{}, &fakeTrustStore{}, &fakeLauncher{}, &fakeClock{now: time.Unix(0, 0)}, nil)
	deps.Operator = operator
	options := testOptions()
	options.AutoSelectOffer = true
	_, err := RunVast(context.Background(), "token", validRecipe(), options, deps)
	if err == nil || !strings.Contains(err.Error(), "cost was not confirmed") {
		t.Fatalf("unexpected result: %v", err)
	}
	if operator.selectCalls != 0 || operator.confirmedView.Offer.HourlyUSD != 2 || operator.confirmedView.MonthlyUSD != 1460 || api.createCalls != 0 {
		t.Fatalf("non-authoritative cost: %+v", operator.confirmedView)
	}
}

func TestRunVastBrowserNeverAuthorizesStaleOrUnpricedSelection(t *testing.T) {
	for _, mode := range []string{"none", "failure", "empty", "cancelled", "unknown price", "refused"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			api := &searchAPI{search: func(ctx context.Context, _ vast.SearchRequest) ([]vast.Offer, error) {
				calls++
				if mode == "failure" && calls == 2 {
					return nil, errors.New("search failed")
				}
				if mode == "empty" && calls == 2 {
					return nil, nil
				}
				return []vast.Offer{{ID: 42, GPUVRAMGB: 96, HourlyUSD: 2, PriceUnknown: mode == "unknown price"}}, nil
			}}
			operator := &browserOperator{fakeOperator: fakeOperator{confirmCost: true}}
			operator.browse = func(ctx context.Context, _ recipe.Recipe, search OfferSearch) (vast.Offer, error) {
				if mode == "none" {
					return vast.Offer{ID: 42}, nil
				}
				queryCtx, cancel := context.WithCancel(ctx)
				defer cancel()
				if mode == "cancelled" {
					cancel()
				}
				_, err := search(queryCtx, OfferQuery{})
				if err != nil && mode != "cancelled" {
					t.Fatal(err)
				}
				if mode == "failure" || mode == "empty" {
					_, _ = search(ctx, OfferQuery{Sort: vast.SortReliability})
				}
				if mode == "refused" {
					return vast.Offer{}, errors.New("browser cancelled")
				}
				return vast.Offer{ID: 42}, nil
			}
			deps := baseDependencies(&api.fakeVastAPI, &operator.fakeOperator, &fakeScanner{}, &fakeTrustStore{}, &fakeLauncher{}, &fakeClock{now: time.Unix(0, 0)}, nil)
			deps.NewAPI = func(string) VastAPI { return api }
			deps.Operator = operator
			_, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
			if err == nil || api.createCalls != 0 || operator.confirmedView.Offer.ID != 0 {
				t.Fatalf("unsafe selection mode %s: err=%v creates=%d confirmed=%+v", mode, err, api.createCalls, operator.confirmedView)
			}
		})
	}
}

func TestRunVastLateSearchCannotReplaceNewerFailure(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	settled := make(chan struct{})
	api := &searchAPI{search: func(ctx context.Context, q vast.SearchRequest) ([]vast.Offer, error) {
		if q.Sort == vast.SortReliability {
			return nil, errors.New("new search failed")
		}
		close(started)
		<-release
		return []vast.Offer{{ID: 42, GPUVRAMGB: 96, HourlyUSD: 1}}, nil
	}}
	operator := &browserOperator{fakeOperator: fakeOperator{confirmCost: true}}
	operator.browse = func(ctx context.Context, _ recipe.Recipe, search OfferSearch) (vast.Offer, error) {
		go func() { defer close(settled); _, _ = search(ctx, OfferQuery{}) }()
		<-started
		_, err := search(ctx, OfferQuery{Sort: vast.SortReliability})
		if err == nil {
			t.Fatal("fixture expected failed latest search")
		}
		close(release)
		<-settled
		return vast.Offer{ID: 42}, nil
	}
	deps := baseDependencies(&api.fakeVastAPI, &operator.fakeOperator, &fakeScanner{}, &fakeTrustStore{}, &fakeLauncher{}, &fakeClock{}, nil)
	deps.NewAPI = func(string) VastAPI { return api }
	deps.Operator = operator
	_, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
	if err == nil || api.createCalls != 0 || operator.confirmedView.Offer.ID != 0 {
		t.Fatalf("stale success authorized cost/create: %v", err)
	}
}

func TestRunVastBrowserReturnCancelsAndAwaitsOutstandingSearch(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	settled := make(chan struct{})
	api := &searchAPI{search: func(ctx context.Context, _ vast.SearchRequest) ([]vast.Offer, error) {
		close(started)
		<-ctx.Done()
		close(cancelled)
		<-release
		close(settled)
		return nil, ctx.Err()
	}}
	operator := &browserOperator{browse: func(ctx context.Context, _ recipe.Recipe, search OfferSearch) (vast.Offer, error) {
		go func() { _, _ = search(ctx, OfferQuery{}) }()
		<-started
		return vast.Offer{}, errors.New("closed browser")
	}}
	deps := baseDependencies(&api.fakeVastAPI, &operator.fakeOperator, &fakeScanner{}, &fakeTrustStore{}, &fakeLauncher{}, &fakeClock{}, nil)
	deps.NewAPI = func(string) VastAPI { return api }
	deps.Operator = operator
	done := make(chan error, 1)
	go func() {
		_, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
		done <- err
	}()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("search was not cancelled")
	}
	select {
	case err := <-done:
		t.Fatalf("returned before search settled: %v", err)
	default:
	}
	close(release)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("missing cancellation error")
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not finish")
	}
	select {
	case <-settled:
	default:
		t.Fatal("search leaked")
	}
}

func TestRunVastPendingQueryInvalidatesPreviouslySuccessfulSelection(t *testing.T) {
	pending := make(chan struct{})
	api := &searchAPI{search: func(ctx context.Context, q vast.SearchRequest) ([]vast.Offer, error) {
		if q.Sort == vast.SortReliability {
			close(pending)
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return []vast.Offer{{ID: 42, GPUVRAMGB: 96, HourlyUSD: 1}}, nil
	}}
	operator := &browserOperator{fakeOperator: fakeOperator{confirmCost: true}, browse: func(ctx context.Context, _ recipe.Recipe, search OfferSearch) (vast.Offer, error) {
		if _, err := search(ctx, OfferQuery{}); err != nil {
			t.Fatal(err)
		}
		go func() { _, _ = search(ctx, OfferQuery{Sort: vast.SortReliability}) }()
		<-pending
		return vast.Offer{ID: 42}, nil
	}}
	deps := baseDependencies(&api.fakeVastAPI, &operator.fakeOperator, &fakeScanner{}, &fakeTrustStore{}, &fakeLauncher{}, &fakeClock{}, nil)
	deps.NewAPI = func(string) VastAPI { return api }
	deps.Operator = operator
	_, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
	if err == nil || api.createCalls != 0 || operator.confirmedView.Offer.ID != 0 {
		t.Fatalf("pending query authorized old result: %v", err)
	}
}

func TestRunVastBrowserExplicitSelectionCreatesOnlyAuthoritativeOffer(t *testing.T) {
	api := &fakeVastAPI{offers: []vast.Offer{{ID: 42, GPUVRAMGB: 96, HourlyUSD: 0}, {ID: 99, GPUVRAMGB: 96, HourlyUSD: 2}}, createID: 123, instances: []vast.Instance{{ID: 123, Status: "running", SSHHost: "gpu.example", SSHPort: 22}}}
	operator := &browserOperator{fakeOperator: fakeOperator{confirmCost: true, confirmKeys: true}, browse: func(ctx context.Context, _ recipe.Recipe, search OfferSearch) (vast.Offer, error) {
		result, err := search(ctx, OfferQuery{})
		if err != nil {
			return vast.Offer{}, err
		}
		result.Views[1].Offer.HourlyUSD = 0
		return vast.Offer{ID: 99, HourlyUSD: 0}, nil
	}}
	deps := baseDependencies(api, &operator.fakeOperator, &fakeScanner{keys: HostKeys{Raw: []byte("fixture"), Fingerprints: []string{"SHA256:fixture"}}}, &fakeTrustStore{}, &fakeLauncher{}, &fakeClock{now: time.Unix(0, 0)}, nil)
	deps.Operator = operator
	result, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
	if err != nil || result.InstanceID != 123 || api.createCalls != 1 || api.createOffer != 99 || operator.confirmedView.Offer.HourlyUSD != 2 {
		t.Fatalf("wrong paid result: %+v %v creates=%d offer=%d", result, err, api.createCalls, api.createOffer)
	}
}

func TestSearchEligibleOffersRejectsInvalidQueriesWithoutAPI(t *testing.T) {
	api := &fakeVastAPI{}
	for _, query := range []OfferQuery{{Countries: []string{"EU"}}, {Sort: "invalid"}} {
		if _, err := SearchEligibleOffers(context.Background(), api, validRecipe(), query, 100); err == nil {
			t.Fatalf("invalid query accepted: %+v", query)
		}
	}
	if api.searchCalls != 0 {
		t.Fatal("invalid query reached API")
	}
}

func TestRunVastCompletedSearchRemainsSelectableAfterRequestCleanup(t *testing.T) {
	api := &fakeVastAPI{offers: []vast.Offer{{ID: 42, GPUVRAMGB: 96, HourlyUSD: 2}}}
	operator := &browserOperator{fakeOperator: fakeOperator{confirmCost: false}, browse: func(ctx context.Context, _ recipe.Recipe, search OfferSearch) (vast.Offer, error) {
		requestCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		result, err := search(requestCtx, OfferQuery{})
		if err != nil {
			return vast.Offer{}, err
		}
		cancel() // normal command/child cleanup after publishing a complete result
		return result.Views[0].Offer, nil
	}}
	deps := baseDependencies(api, &operator.fakeOperator, &fakeScanner{}, &fakeTrustStore{}, &fakeLauncher{}, &fakeClock{}, nil)
	deps.Operator = operator
	_, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
	if err == nil || !strings.Contains(err.Error(), "cost was not confirmed") || operator.confirmedView.Offer.ID != 42 {
		t.Fatalf("request cleanup discarded completed selection: %v", err)
	}
}

func TestRunVastDelayedCanceledSearchPreservesNewerSelection(t *testing.T) {
	api := &fakeVastAPI{offers: []vast.Offer{{ID: 42, GPUVRAMGB: 96, HourlyUSD: 2}}}
	operator := &browserOperator{fakeOperator: fakeOperator{confirmCost: false}, browse: func(ctx context.Context, _ recipe.Recipe, search OfferSearch) (vast.Offer, error) {
		oldCtx, cancelOld := context.WithCancel(ctx)
		cancelOld()
		result, err := search(ctx, OfferQuery{Sort: vast.SortReliability})
		if err != nil {
			return vast.Offer{}, err
		}
		// A canceled Tea command can start after a newer command has completed.
		if _, err := search(oldCtx, OfferQuery{}); !errors.Is(err, context.Canceled) {
			t.Fatalf("delayed canceled request returned %v", err)
		}
		return result.Views[0].Offer, nil
	}}
	deps := baseDependencies(api, &operator.fakeOperator, &fakeScanner{}, &fakeTrustStore{}, &fakeLauncher{}, &fakeClock{}, nil)
	deps.Operator = operator
	_, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
	if err == nil || !strings.Contains(err.Error(), "cost was not confirmed") || operator.confirmedView.Offer.ID != 42 || operator.confirmedView.Offer.HourlyUSD != 2 || api.createCalls != 0 {
		t.Fatalf("delayed canceled request discarded authoritative selection: view=%+v err=%v", operator.confirmedView, err)
	}
}
