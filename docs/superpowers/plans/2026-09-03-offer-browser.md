# Visual Offer Browser Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Steps use checkbox syntax for tracking.

**Goal:** Replace the capacity radar with explicit, visual offer selection and server-side country filtering for every recipe.

**Architecture:** A shared setup search service owns recipe eligibility. An optional browser operator receives a cancellable search function, without credentials, and returns an offer ID validated against the current successful search. A dedicated embedded CLI model owns browsing, filter drafts, details and search generations; the root still owns the single terminal session and paid confirmation.

**Tech Stack:** Existing Go, Bubble Tea, Bubbles, Huh, Lip Gloss and x/text dependencies. No web interface or new graphical dependency.

## Global Constraints

- Toutes les recettes passent par un choix explicite d’offre, y compris les recettes intégrées.
- Aucun nouvel essai automatique de création. Aucun appel réel au fournisseur pendant les tests.
- Codes ISO à deux lettres normalisés, sans doublon ; liste vide = tous pays.
- Charger jusqu’à 100 offres par recherche. Afficher le nombre chargé, sans prétendre connaître le total du marché.
- Les anciens résultats ne sont pas sélectionnables pendant son chargement. Un résultat tardif ne peut pas remplacer la recherche courante.
- Inconnu signifie inconnu, jamais zéro. Pas de score composite ni radar.
- Sous 30×10, aucune action invisible ne peut être validée.
- « Annuler » reste le choix par défaut. Facturation immédiate, calcul seul, stockage / trafic sortant / taxes exclus.
- Préserver remplacement, identité, empreintes SSH, création incertaine, absence de destruction distante et session unique.
- Preserve the dirty working tree. Work in the existing feature checkout, with task snapshots/reports instead of committing pre-existing changes. Prefix every shell command with rtk.

## Review decisions

- Query filtering belongs before the provider limit, never only in the visible list.
- Keep source presence for price/reliability: missing JSON values must not become a free offer or a zero reliability assertion. Unknown price is not rentable.
- Validate country membership against an independent ISO catalog; do not infer choices from loaded offers. Provider location parsing is conservative and raw location remains inspectable.
- The callback's current eligible set must be mutex/generation protected. Clear it at search start, reject canceled/late completions, and close it when browsing returns. Re-resolve the selected ID to provider facts before confirmation.
- Never replay a browser choice after Back; restore query drafts but require fresh explicit selection. Root cancellation closes outstanding searches; filter input must not trigger root q-to-quit.

### Task 1: Provider query and shared eligible search

**Files:** modify `internal/vast/search.go`, `internal/setup/orchestrator.go`; create `internal/vast/countries.go`, `internal/setup/offers.go`; tests beside each.

**Interfaces:**
```go
// internal/vast
type OfferSort string // SortPrice = "price", SortReliability = "reliability"
// SearchRequest adds Countries []string and Sort OfferSort.
func NormalizeCountries([]string) ([]string, error)
type Country struct { Code, Name string }
func Countries() []Country
func CountryCode(location string) string
// Offer adds PriceUnknown, ReliabilityUnknown bool; legacy literal positive values remain known.
// internal/setup
type OfferQuery struct { Countries []string; Sort vast.OfferSort }
type OfferSearchResult struct { Views []OfferView; Loaded int; Limit int }
type OfferSearch func(context.Context, OfferQuery) (OfferSearchResult, error)
type OfferBrowser interface { BrowseOffers(context.Context, recipe.Recipe, OfferSearch) (vast.Offer, error) }
func SearchEligibleOffers(context.Context, VastAPI, recipe.Recipe, OfferQuery, int) (OfferSearchResult, error)
```

- [x] Add failing HTTP payload tests for FR/DE normalization and duplicate removal, all-countries omission, invalid codes/sort, reliability then price order, limit100 and unchanged hardware constraints. Add decode tests for missing versus explicit zero price/reliability.
```go
// Request normalization example; JSON request must contain ["FR","DE"].
request.Countries = []string{" fr ", "DE", "fr"}
request.Sort = vast.SortReliability
// Assert server observes geolocation.in and order [[reliability,desc],[dph_total,asc]].
```
- [x] Run `rtk go test ./internal/vast -count=1`; record expected new-test failures.
- [x] Implement validated serialization and an independent ISO catalog (existing x/text region/display support or static catalog). Preserve price/reliability presence using pointer response fields; reject non-finite/negative price as unavailable.
- [x] Add failing setup tests: shared strict/preferred hardware checks; browser called even with AutoSelectOffer legacy flag; selected forged fields replaced with authoritative facts; stale/canceled/error result cannot authorize Create; no Create on inspection/refusal; fallback SelectOffer still works explicitly.
```go
// Search service returns empty Views without error, preserving Loaded for the UI.
// Browser seam selection must resolve through the latest successful search ID.
// Unknown-price offers are inspectable but are rejected before paid confirmation.
```
- [x] Extract filtering from RunVast into SearchEligibleOffers. Sorting respects query with deterministic ID tie-break. RunVast uses optional OfferBrowser callback with synchronized current results, then separate existing ConfirmCost. Legacy non-browser operators get explicit SelectOffer without automatic fallback.
- [x] Run `rtk go test ./internal/vast ./internal/setup -count=1`; report RED/GREEN and changed files. Save task diff snapshot, no mixed commit.

### Task 2: Embedded visual browser, accessible flow and useful review

**Files:** create `internal/cli/offer_browser.go`, `offer_browser_view.go`, `offer_facts.go` and tests; modify `application.go`, `application_prompts.go`, `application_navigation.go`, `application_view.go`, `accessible.go`, `prompts.go`, `setup.go`, `rental_review.go`, `cmd/sovkit/main.go`, related tests, README.

**Consumes:** Task1 OfferQuery/OfferSearch/OfferSearchResult/OfferBrowser and Vast country/sort APIs exactly as declared above.

**Internal interface:**
```go
type offerBrowserPrompt struct { ctx context.Context; recipe recipe.Recipe; search setup.OfferSearch }
// New promptBrowser is never replayed; root handles browserSelectedMsg / browserBackMsg.
func (p *applicationPrompter) BrowseOffers(context.Context, recipe.Recipe, setup.OfferSearch) (vast.Offer, error)
func (p *AccessiblePrompter) BrowseOffers(context.Context, recipe.Recipe, setup.OfferSearch) (vast.Offer, error)
func (p *HuhPrompter) BrowseOffers(context.Context, recipe.Recipe, setup.OfferSearch) (vast.Offer, error)
// Browser model stores cancel, generation, query, filter draft, selectedID, views,
// viewport and finite transition state. No API key and no create dependency.
```

- [x] Write state-transition tests first: initial/refresh loading disables Enter, late responses ignored, filters retain union on error/empty, cancel filter discards edits, selected ID only retained when present, sorting invokes search, browsing emits selection only explicitly.
- [x] Run focused CLI tests to observe failures before implementation.
- [x] Implement embedded Bubble Tea browser: cap overall width around150 cells; left offer rows and right detailed card on wide screens, list/detail navigation on narrow screens. Use aligned bordered panels, cyan emphasis, amber monetary values, textual cursor, exact capacity bars and requirement markers, no radar. Render provider reliability as declared, not guaranteed. Price/monthly based on730hours, unknowns textual. Animate bars briefly on selection; numeric values immediate, no loop.
- [x] Implement searchable country multiselect using Huh or existing Bubbles list/textinput, independent ISO catalog, Space toggle, Enter Apply, Esc cancel; separate draft from applied query. All countries resets empty. f/s/r/i/Enter/Esc and fixed contextual help remain usable on30×10. Long details use viewport. Browser cancels search on close, root back and shutdown; reject outdated child messages.
- [x] Add failing root integration tests for q while filter typing, Esc within details/filter, fresh browser on Back, no creation on navigation/filtering/inspection, separate default-cancel confirmation. Root uses browser-specific footer instead of duplicating controls; no new tea.Program in root path.
- [x] Wire optional BrowseOffers through production prompters. All builtins AutoSelectOffer false; main OfferLimit100. Text mode loops numbered offers with f countries (comma-separated validated ISO codes), s sort, r refresh, i inspect and explicit selection. Empty/error allows controls; EOF returns without mutation. Noninteractive paths preserve existing secret handling.
- [x] Add failing review tests for real capacity bars and no radar, unknown values and multi-GPU units. Replace the review radar/scan rendering with the shared facts/capacity helpers; keep fixed create/cancel controls, default cancel, inspectable long metadata and exact billing warning. Remove obsolete radar tests/helpers rather than retaining a hidden unused visualization.
- [x] Run targeted tests, then full suite; update old tests only where explicit selection/new layout intentionally changes expectations. Document new controls and source/price limitations in README.
- [x] Record actual renders at30×10,72×24,108×30,150×34,320×80 from fake offers; verify no horizontal/vertical overflow, readable selected row, pinned actions, full details reachable, rapid resize and filtering. Save a test-only PTY fixture if useful; no production debug backdoor.
- [x] Run `rtk go test ./... -count=1`, `rtk go test -race ./internal/cli ./internal/setup ./internal/vast -count=1`, `rtk go vet ./...`, `rtk git diff --check`. Save report and full task diff snapshot, no mixed commit.

### Completion

- [x] Independent final review of task diffs and approved spec; resolve concrete findings and rerun covering tests.
- [x] Confirm no real instance/API action occurred and report user-visible behavior plus actual verification evidence.
