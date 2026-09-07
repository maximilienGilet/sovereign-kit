package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/maximilienGilet/sovereign-kit/internal/catalogui"
	"github.com/maximilienGilet/sovereign-kit/internal/huggingface"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

type promptKind string

const (
	promptProvider        promptKind = "Provider"
	promptManual          promptKind = "Manual SSH"
	promptWorkload        promptKind = "Recipe"
	promptQuery           promptKind = "Hugging Face search"
	promptModel           promptKind = "Hugging Face model"
	promptHardware        promptKind = "Hardware"
	promptCustom          promptKind = "Custom workload review"
	promptToken           promptKind = "Vast API key"
	promptIdentity        promptKind = "SSH identity"
	promptIdentityConsent promptKind = "Identity approval"
	promptOffer           promptKind = "Offer"
	promptBrowser         promptKind = "Browse offers"
	promptCost            promptKind = "Deployment review"
)

// Requests contain data, never form pointers. Only the root owns editors.
type promptRequest struct {
	kind  promptKind
	data  any
	reply chan any
}
type promptAnswer struct {
	kind  promptKind
	value any
}
type costPrompt struct {
	view setup.OfferView
	disk int
}
type offerBrowserPrompt struct {
	ctx    context.Context
	recipe recipe.Recipe
	search setup.OfferSearch
}
type identityPrompt struct {
	path     string
	generate bool
}
type customPrompt struct {
	model    huggingface.Model
	hardware CustomHardware
}
type applicationEvent struct {
	generation uint64
	value      any
}
type applicationDone struct{ err error }

type applicationSession struct {
	generation uint64
	ctx        context.Context
	cancel     context.CancelFunc
	events     chan any
	done       chan struct{}
	err        error // published by close(done)
	mu         sync.Mutex
	progress   setup.Progress // preserved even when cancellation discards UI events
	activity   setup.Activity
	logs       setup.ServerLogs
	creating   bool
	recovery   setup.InstanceRecovery
}

func (s *applicationSession) next() tea.Cmd {
	return func() tea.Msg {
		var value any
		select {
		case value = <-s.events:
		case <-s.done:
			select {
			case value = <-s.events:
			default:
				value = applicationDone{s.err}
			}
		}
		return applicationEvent{s.generation, value}
	}
}

type applicationPrompter struct{ session *applicationSession }

func (p *applicationPrompter) ask(ctx context.Context, kind promptKind, data any) (any, error) {
	request := promptRequest{kind: kind, data: data, reply: make(chan any, 1)}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case p.session.events <- request:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case answer := <-request.reply:
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return answer, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func promptValue[T any](p *applicationPrompter, ctx context.Context, kind promptKind, data any) (T, error) {
	var zero T
	result, err := p.ask(ctx, kind, data)
	if err != nil {
		return zero, err
	}
	value, ok := result.(T)
	if !ok {
		return zero, fmt.Errorf("invalid answer for %s", kind)
	}
	return value, nil
}
func (p *applicationPrompter) SelectProvider(ctx context.Context) (string, error) {
	return promptValue[string](p, ctx, promptProvider, nil)
}
func (p *applicationPrompter) ManualRoute(ctx context.Context, user string) (ManualRoute, error) {
	return promptValue[ManualRoute](p, ctx, promptManual, user)
}
func (p *applicationPrompter) SelectWorkload(ctx context.Context, rs []recipe.Recipe) (string, error) {
	return promptValue[string](p, ctx, promptWorkload, append([]recipe.Recipe(nil), rs...))
}
func (p *applicationPrompter) VastAPIKey(ctx context.Context) (string, error) {
	return promptValue[string](p, ctx, promptToken, nil)
}
func (p *applicationPrompter) VastIdentity(ctx context.Context, path string) (string, error) {
	return promptValue[string](p, ctx, promptIdentity, path)
}
func (p *applicationPrompter) HuggingFaceQuery(ctx context.Context) (string, error) {
	return promptValue[string](p, ctx, promptQuery, nil)
}
func (p *applicationPrompter) SelectHuggingFaceModel(ctx context.Context, rs []huggingface.SearchResult) (string, error) {
	return promptValue[string](p, ctx, promptModel, append([]huggingface.SearchResult(nil), rs...))
}
func (p *applicationPrompter) CustomHardware(ctx context.Context) (CustomHardware, error) {
	return promptValue[CustomHardware](p, ctx, promptHardware, nil)
}
func (p *applicationPrompter) ConfirmCustomWorkload(ctx context.Context, model huggingface.Model, hardware CustomHardware) (bool, error) {
	return promptValue[bool](p, ctx, promptCustom, customPrompt{model, hardware})
}
func (p *applicationPrompter) SelectOffer(ctx context.Context, views []setup.OfferView) (vast.Offer, error) {
	return promptValue[vast.Offer](p, ctx, promptOffer, append([]setup.OfferView(nil), views...))
}
func (p *applicationPrompter) BrowseOffers(ctx context.Context, r recipe.Recipe, search setup.OfferSearch) (vast.Offer, error) {
	return promptValue[vast.Offer](p, ctx, promptBrowser, offerBrowserPrompt{ctx, r, search})
}
func (p *applicationPrompter) ConfirmCost(ctx context.Context, view setup.OfferView, disk int) (bool, error) {
	return promptValue[bool](p, ctx, promptCost, costPrompt{view, disk})
}
func (p *applicationPrompter) ConfirmIdentitySetup(ctx context.Context, path string, generate bool) (bool, error) {
	return promptValue[bool](p, ctx, promptIdentityConsent, identityPrompt{path, generate})
}
func (p *applicationPrompter) SetupProgress(progress setup.Progress) {
	s := p.session
	s.mu.Lock()
	if progress.Stage == setup.ProgressCreating {
		s.creating = true
	}
	if progress.InstanceID > 0 || s.progress.InstanceID == 0 {
		s.progress = progress
	}
	s.mu.Unlock()
	select {
	case s.events <- progress:
	case <-s.ctx.Done():
	}
}

type promptEditor struct {
	text      string
	number    int
	route     ManualRoute
	port      string
	vram      string
	disk      string
	confirmed bool
}

func (m *applicationModel) openPrompt(request promptRequest) tea.Cmd {
	m.pending = &request
	m.screen = "prompt"
	m.childGeneration++
	m.confirmText = ""
	m.resizeFormDescription = nil
	m.recipeViewport.GotoTop()
	m.editor = &promptEditor{port: "22"}
	e := m.editor
	draft := m.drafts[request.kind]
	switch request.kind {
	case promptWorkload, promptModel, promptOffer:
		if !validReplay(request, promptAnswer{request.kind, draft}) {
			draft = nil
		}
	}
	if value, ok := draft.(string); ok {
		e.text = value
	}
	if value, ok := draft.(ManualRoute); ok {
		e.route = value
		e.port = strconv.Itoa(value.Port)
	}
	if value, ok := draft.(CustomHardware); ok {
		e.vram = strconv.Itoa(value.MinimumVRAMGB)
		e.disk = strconv.Itoa(value.MinimumDiskGB)
	}
	if value, ok := draft.(promptEditor); ok {
		*e = value
	}
	var fields []huh.Field
	switch request.kind {
	case promptProvider:
		field := providerSelectField(&e.text)
		m.responsiveDescription(func(value string) { field.Description(value) }, providerDescription)
		fields = []huh.Field{field}
	case promptManual:
		user := request.data.(string)
		if e.route.User != "" {
			user = e.route.User
		}
		fields = manualRouteFields(&e.route, &e.port, user)
		fields[1].(*huh.Input).Validate(func(value string) error {
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 || n > 65535 {
				return fmt.Errorf("SSH port must be 1–65535")
			}
			return nil
		})
	case promptWorkload:
		entries := workloadEntries(request.data.([]recipe.Recipe))
		picker := catalogui.NewEmbeddedPicker(entries)
		// Restore by stable ID, not a cached index into potentially refreshed data.
		for i, entry := range entries {
			if entry.Value == e.text {
				for j := 0; j < i; j++ {
					updated, _ := picker.Update(tea.KeyMsg{Type: tea.KeyDown})
					picker = updated.(catalogui.Model)
				}
				break
			}
		}
		m.child = picker
	case promptQuery:
		field := huggingFaceQueryField(&e.text)
		m.responsiveDescription(func(value string) { field.Description(value) }, huggingFaceQueryDescription)
		fields = []huh.Field{field}
	case promptModel:
		fields = []huh.Field{huh.NewSelect[string]().Title("Choose a Hugging Face model").Options(huggingFaceModelOptions(request.data.([]huggingface.SearchResult))...).Value(&e.text)}
	case promptHardware:
		fields = customHardwareFields(&e.vram, &e.disk)
		for _, field := range fields {
			field.(*huh.Input).Validate(func(value string) error {
				n, err := strconv.Atoi(value)
				if err != nil || n < 1 {
					return fmt.Errorf("Enter a positive number")
				}
				return nil
			})
		}
	case promptToken:
		field := vastAPIKeyField(&e.text)
		m.responsiveDescription(func(value string) { field.Description(value) }, vastAPIKeyDescription)
		fields = []huh.Field{field}
	case promptResumeID:
		fields = []huh.Field{huh.NewInput().Title("Existing Vast instance ID").Description("Find Instance ID in Vast. This does not create a new instance.").Value(&e.text).Validate(func(value string) error { _, err := parseResumeID(value); return err })}
	case promptIdentity:
		if e.text == "" {
			e.text = request.data.(string)
		}
		field := vastIdentityField(&e.text)
		m.responsiveDescription(func(value string) { field.Description(value) }, vastIdentityDescription)
		fields = []huh.Field{field}
	case promptOffer:
		views := request.data.([]setup.OfferView)
		if value, ok := draft.(vast.Offer); ok {
			e.number = value.ID
		}
		fields = []huh.Field{huh.NewSelect[int]().Title("Choose a Vast GPU offer").Options(offerOptions(views)...).Value(&e.number)}
	case promptBrowser:
		data := request.data.(offerBrowserPrompt)
		browser := newOfferBrowser(data.ctx, data.recipe, data.search)
		if m.offerQuery != nil {
			browser.query = *m.offerQuery
			browser.query.Countries = append([]string(nil), m.offerQuery.Countries...)
		}
		m.child = browser
	case promptCost:
		data := request.data.(costPrompt)
		m.child = newEmbeddedRentalReview(data.view, data.disk)
	case promptCustom:
		data := request.data.(customPrompt)
		m.setConfirmation(customWorkloadConfirmationTitle(data.model, data.hardware))
	case promptIdentityConsent:
		data := request.data.(identityPrompt)
		m.setConfirmation(identitySetupTitle(data.path, data.generate))
	case promptResume:
		m.setConfirmation(request.data.(string))
	}
	if len(fields) > 0 {
		keymap := setupKeyMap()
		if request.kind == promptProvider {
			keymap = providerKeyMap()
		}
		m.child = huh.NewForm(huh.NewGroup(fields...)).WithKeyMap(keymap).WithShowHelp(false)
	}
	m.resizeChild()
	if m.child == nil {
		return nil
	}
	return m.tagChild(m.child.Init())
}

// Descriptive guidance is secondary to the active control on short screens.
// Mutate the existing Huh field so resizing preserves its focus and value.
func (m *applicationModel) responsiveDescription(set func(string), description string) {
	m.resizeFormDescription = func(compact bool) {
		if compact {
			set("")
		} else {
			set(description)
		}
	}
}

func (m *applicationModel) editorAnswer() (any, error) {
	e := m.editor
	switch m.pending.kind {
	case promptManual:
		e.route.Port, _ = strconv.Atoi(e.port)
		return e.route, nil
	case promptHardware:
		vram, _ := strconv.Atoi(e.vram)
		disk, _ := strconv.Atoi(e.disk)
		return CustomHardware{MinimumVRAMGB: vram, MinimumDiskGB: disk}, nil
	case promptCustom, promptIdentityConsent, promptResume:
		return e.confirmed, nil
	case promptOffer:
		for _, view := range m.pending.data.([]setup.OfferView) {
			if view.Offer.ID == e.number {
				return view.Offer, nil
			}
		}
		return nil, fmt.Errorf("selected offer is unavailable")
	default:
		return strings.TrimSpace(e.text), nil
	}
}

// Selection replay is checked against fresh provider results. Confirmations are never replayed.
func validReplay(request promptRequest, answer promptAnswer) bool {
	if request.kind != answer.kind {
		return false
	}
	switch request.kind {
	case promptCost, promptCustom, promptIdentityConsent, promptManual, promptBrowser, promptResume:
		return false
	case promptProvider:
		return answer.value == "vast" || answer.value == "manual"
	case promptWorkload:
		for _, entry := range workloadEntries(request.data.([]recipe.Recipe)) {
			if entry.Value == answer.value {
				return true
			}
		}
		return false
	case promptModel:
		for _, model := range request.data.([]huggingface.SearchResult) {
			if model.Repository == answer.value {
				return true
			}
		}
		return false
	case promptOffer:
		offer, ok := answer.value.(vast.Offer)
		if !ok {
			return false
		}
		for _, view := range request.data.([]setup.OfferView) {
			if view.Offer.ID == offer.ID {
				return true
			}
		}
		return false
	default:
		return true
	}
}
