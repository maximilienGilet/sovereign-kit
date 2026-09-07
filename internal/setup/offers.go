package setup

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

type OfferQuery struct {
	Region    string
	Countries []string
	Sort      vast.OfferSort
}
type OfferSearchResult struct {
	Views         []OfferView
	Loaded, Limit int
}
type OfferSearch func(context.Context, OfferQuery) (OfferSearchResult, error)
type OfferBrowser interface {
	BrowseOffers(context.Context, recipe.Recipe, OfferSearch) (vast.Offer, error)
}

// SearchEligibleOffers is the shared read-only eligibility boundary. An empty
// result remains a successful search so a browser can edit its query.
func SearchEligibleOffers(ctx context.Context, api VastAPI, r recipe.Recipe, query OfferQuery, limit int) (OfferSearchResult, error) {
	if err := ctx.Err(); err != nil {
		return OfferSearchResult{}, err
	}
	if err := r.Validate(); err != nil {
		return OfferSearchResult{}, err
	}
	if api == nil {
		return OfferSearchResult{}, fmt.Errorf("Vast API is required")
	}
	if limit == 0 {
		limit = 5
	}
	if limit < 1 || limit > 100 {
		return OfferSearchResult{}, fmt.Errorf("offer limit must be between 1 and 100")
	}
	countries, err := vast.GeographicCountries(query.Region, query.Countries)
	if err != nil {
		return OfferSearchResult{}, err
	}
	if query.Sort != "" && query.Sort != vast.SortPrice && query.Sort != vast.SortReliability {
		return OfferSearchResult{}, fmt.Errorf("invalid offer sort %q", query.Sort)
	}
	request := vast.SearchRequest{Limit: limit, Countries: countries, Sort: query.Sort, GPUModel: r.Requirements.GPUModel, GPUCount: r.Requirements.GPUCount, StrictGPU: r.Requirements.StrictGPU, MinimumVRAMGB: r.Requirements.MinimumVRAMGB, MinimumDiskGB: r.Requirements.MinimumDiskGB}
	offers, err := api.SearchOffers(ctx, request)
	if err != nil {
		return OfferSearchResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return OfferSearchResult{}, err
	}
	result := OfferSearchResult{Loaded: len(offers), Limit: limit, Views: make([]OfferView, 0, len(offers))}
	for _, offer := range offers {
		if offer.GPUVRAMGB < float64(r.Requirements.MinimumVRAMGB) {
			continue
		}
		if offer.DiskSpaceGB > 0 && offer.DiskSpaceGB < float64(r.Requirements.MinimumDiskGB) {
			continue
		}
		if r.Requirements.StrictGPU && (strings.TrimSpace(offer.GPUName) != strings.TrimSpace(r.Requirements.GPUModel) || offer.GPUCount != r.Requirements.GPUCount) {
			continue
		}
		if r.Requirements.StrictGPU && offer.TotalGPUVRAMGB > 0 && offer.TotalGPUVRAMGB < float64(r.Requirements.MinimumVRAMGB*r.Requirements.GPUCount) {
			continue
		}
		if len(countries) > 0 {
			matches := false
			for _, country := range countries {
				if vast.CountryCode(offer.Location) == country {
					matches = true
					break
				}
			}
			if !matches {
				continue
			}
		}
		if offer.HourlyUSD < 0 || math.IsNaN(offer.HourlyUSD) || math.IsInf(offer.HourlyUSD, 0) {
			offer.PriceUnknown = true
			offer.HourlyUSD = 0
		}
		if offer.Reliability < 0 || offer.Reliability > 1 || math.IsNaN(offer.Reliability) {
			offer.ReliabilityUnknown = true
			offer.Reliability = 0
		}
		result.Views = append(result.Views, viewFor(offer, cloneOfferRecipe(r)))
	}
	sort.SliceStable(result.Views, func(i, j int) bool {
		a, b := result.Views[i].Offer, result.Views[j].Offer
		if query.Sort == vast.SortReliability {
			if a.ReliabilityUnknown != b.ReliabilityUnknown {
				return !a.ReliabilityUnknown
			}
			if !a.ReliabilityUnknown && a.Reliability != b.Reliability {
				return a.Reliability > b.Reliability
			}
		}
		if a.PriceUnknown != b.PriceUnknown {
			return !a.PriceUnknown
		}
		if !a.PriceUnknown && a.HourlyUSD != b.HourlyUSD {
			return a.HourlyUSD < b.HourlyUSD
		}
		return a.ID < b.ID
	})
	return result, nil
}

func cloneOfferRecipe(r recipe.Recipe) recipe.Recipe {
	r.Profile.UseWhen = append([]string(nil), r.Profile.UseWhen...)
	if r.Speculative != nil {
		speculative := *r.Speculative
		r.Speculative = &speculative
	}
	return r
}
func cloneOfferViews(views []OfferView) []OfferView {
	copy := append([]OfferView(nil), views...)
	for i := range copy {
		copy[i].Recipe = cloneOfferRecipe(copy[i].Recipe)
	}
	return copy
}

func chooseEligibleOffer(ctx context.Context, api VastAPI, r recipe.Recipe, limit int, operator Operator, token string) (OfferView, error) {
	if browser, ok := operator.(OfferBrowser); ok {
		return browseEligibleOffer(ctx, api, r, limit, browser, token)
	}
	result, err := SearchEligibleOffers(ctx, api, r, OfferQuery{}, limit)
	if err != nil {
		return OfferView{}, err
	}
	if len(result.Views) == 0 {
		return OfferView{}, fmt.Errorf("no eligible Vast offers found")
	}
	selected, err := operator.SelectOffer(ctx, cloneOfferViews(result.Views))
	if err != nil {
		return OfferView{}, err
	}
	if err := ctx.Err(); err != nil {
		return OfferView{}, err
	}
	view, ok := selectedView(result.Views, selected.ID)
	if !ok {
		return OfferView{}, fmt.Errorf("selected Vast offer %d is not eligible", selected.ID)
	}
	return view, nil
}

// Each new query invalidates the previous snapshot before starting IO. The
// browser never owns credentials or the API; only this read-only callback.
func browseEligibleOffer(ctx context.Context, api VastAPI, r recipe.Recipe, limit int, browser OfferBrowser, token string) (OfferView, error) {
	sessionCtx, cancelSession := context.WithCancel(ctx)
	defer cancelSession()
	var mu sync.Mutex
	var workers sync.WaitGroup
	var generation uint64
	var closed bool
	var current []OfferView
	var cancelPrevious context.CancelFunc
	search := func(caller context.Context, query OfferQuery) (OfferSearchResult, error) {
		mu.Lock()
		if closed {
			mu.Unlock()
			return OfferSearchResult{}, context.Canceled
		}
		// A delayed, already-canceled command must not supersede a newer query.
		if err := caller.Err(); err != nil {
			mu.Unlock()
			return OfferSearchResult{}, err
		}
		generation++
		id := generation
		current = nil
		if cancelPrevious != nil {
			cancelPrevious()
		}
		requestCtx, cancelRequest := context.WithCancel(sessionCtx)
		cancelPrevious = cancelRequest
		workers.Add(1)
		mu.Unlock()
		defer workers.Done()
		defer cancelRequest()
		stopCaller := context.AfterFunc(caller, cancelRequest)
		defer stopCaller()
		if caller.Err() != nil {
			cancelRequest()
		}
		result, err := SearchEligibleOffers(requestCtx, api, r, query, limit)
		// Browsers show retryable diagnostics before RunVast returns. Redact at
		// this boundary; neither visual nor text browser receives credentials.
		if err != nil && token != "" && strings.Contains(err.Error(), token) {
			err = &redactedOfferSearchError{err, token}
		}
		mu.Lock()
		defer mu.Unlock()
		if err == nil {
			err = caller.Err()
		}
		if err == nil {
			err = requestCtx.Err()
		}
		if closed || generation != id {
			if err == nil {
				err = context.Canceled
			}
		}
		if err != nil {
			return OfferSearchResult{}, err
		}
		current = cloneOfferViews(result.Views)
		return result, nil
	}
	selected, err := browser.BrowseOffers(ctx, cloneOfferRecipe(r), search)
	mu.Lock()
	closed = true // serializes Wait with all possible Add calls
	views := current
	mu.Unlock()
	cancelSession()
	workers.Wait()
	if err != nil {
		return OfferView{}, err
	}
	if err := ctx.Err(); err != nil {
		return OfferView{}, err
	}
	// Caller request cancellation after publication is ordinary IO cleanup.
	// Only a new query or parent session cancellation invalidates this snapshot.
	view, ok := selectedView(views, selected.ID)
	if !ok {
		return OfferView{}, fmt.Errorf("selected Vast offer %d is not in the current eligible result", selected.ID)
	}
	return view, nil
}

type redactedOfferSearchError struct {
	cause error
	token string
}

func (e *redactedOfferSearchError) Error() string {
	return strings.ReplaceAll(e.cause.Error(), e.token, "[redacted]")
}
func (e *redactedOfferSearchError) Unwrap() error { return e.cause }
