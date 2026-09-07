package cli

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/catalog"
	"github.com/maximilienGilet/sovereign-kit/internal/catalogui"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/huggingface"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

type applicationAPI struct {
	creates atomic.Int32
	search  func(context.Context) ([]vast.Offer, error)
	create  func(context.Context) (int, error)
}

func (a *applicationAPI) SearchOffers(ctx context.Context, _ vast.SearchRequest) ([]vast.Offer, error) {
	if a.search != nil {
		return a.search(ctx)
	}
	return []vast.Offer{{ID: 42, GPUName: "test", GPUCount: 1, GPUVRAMGB: 200, DiskSpaceGB: 1000, HourlyUSD: 1}}, nil
}
func (a *applicationAPI) CreateInstance(ctx context.Context, _ int, _ vast.CreateRequest) (int, error) {
	a.creates.Add(1)
	if a.create != nil {
		return a.create(ctx)
	}
	return 987, nil
}
func (a *applicationAPI) GetInstance(context.Context, int) (vast.Instance, error) {
	return vast.Instance{ID: 987, Status: "running", SSHHost: "gpu.example", SSHPort: 22}, nil
}

type applicationScanner struct{}

func (applicationScanner) Scan(context.Context, string, int) (setup.HostKeys, error) {
	return setup.HostKeys{Raw: []byte("host-key"), Fingerprints: []string{"SHA256:real-fixture"}}, nil
}

type applicationTrust struct{}

func (applicationTrust) Save(string, []byte) error { return nil }

type applicationServer struct{}

func (applicationServer) Launch(context.Context, config.SSH, recipe.Recipe) error { return nil }

func vastApplication(t *testing.T, api *applicationAPI) *applicationModel {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	deps := ApplicationDependencies{Setup: SetupDependencies{Recipes: []recipe.Recipe{testRecipe()}, Getenv: func(string) string { return "env-very-secret" }, HomeDir: func() (string, error) { return t.TempDir(), nil }}}
	deps.Setup.RunVast = func(ctx context.Context, token, identity string, w VastWorkload, operator setup.Operator) (setup.Result, error) {
		return setup.RunVast(ctx, token, w.Recipe, setup.Options{ConfigPath: path, IdentityFile: identity, KnownHostsDir: t.TempDir(), AutoSelectOffer: w.AutoSelectOffer, PollTimeout: time.Minute}, setup.Dependencies{
			NewAPI: func(string) setup.VastAPI { return api }, Operator: operator, HostKeyScanner: applicationScanner{}, TrustStore: applicationTrust{}, ServerLauncher: applicationServer{}, Clock: setup.RealClock{}, SaveConfig: config.Save, ValidateIdentity: func(string) error { return nil },
		})
	}
	m := newApplication(context.Background(), path, "root", "setup", deps)
	t.Cleanup(m.cleanup)
	m.Update(m.Init()())
	return m
}
func advanceVastToCost(t *testing.T, m *applicationModel) {
	t.Helper()
	nextApplicationPrompt(t, m, promptProvider)
	m.acceptPrompt("vast")
	nextApplicationPrompt(t, m, promptWorkload)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("embedded picker did not return selection")
	}
	updateApplicationCommand(t, m, cmd)
	if m.quitting {
		t.Fatal("embedded picker quit root")
	}
	nextApplicationPrompt(t, m, promptIdentity)
	m.acceptPrompt("/fixture/key")
	chooseApplicationOffer(t, m)
	nextApplicationPrompt(t, m, promptCost)
}

// Root event helpers do not run arbitrary Tea batches (one branch waits for
// worker input). Explicitly execute the browser's read-only search and choice.
func chooseApplicationOffer(t *testing.T, m *applicationModel) {
	t.Helper()
	nextApplicationPrompt(t, m, promptBrowser)
	browser := m.child.(*offerBrowserModel)
	m.Update(m.tagChild(browser.refresh())())
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 34})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("browser did not return explicit selection")
	}
	updateApplicationCommand(t, m, cmd)
}

func TestApplicationVastRepeatedSubmitNeverRecreates(t *testing.T) {
	api := &applicationAPI{}
	m := vastApplication(t, api)
	advanceVastToCost(t, m)
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	_, first := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_, duplicate := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updateApplicationCommand(t, m, first)
	updateApplicationCommand(t, m, duplicate)
	if !m.locked {
		t.Fatal("cost acceptance did not lock replay")
	}
	for m.session != nil {
		nextApplicationEvent(t, m)
	}
	if api.creates.Load() != 1 || m.instanceID != 987 || m.screen != "connecting" {
		t.Fatalf("paid outcome: creates=%d id=%d view=%s", api.creates.Load(), m.instanceID, m.View())
	}
}

func TestApplicationCancellationRetainsLateKnownInstance(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	api := &applicationAPI{create: func(context.Context) (int, error) { close(started); <-release; return 987, nil }}
	m := vastApplication(t, api)
	advanceVastToCost(t, m)
	m.acceptPrompt(true)
	<-started
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	close(release)
	for m.session != nil {
		nextApplicationEvent(t, m)
	}
	if m.instanceID != 987 || !m.creating || api.creates.Load() != 1 {
		t.Fatalf("cancel lost paid ID: %d %v", m.instanceID, m.creating)
	}
}

func TestApplicationUncertainCreationHasNoReplayAndRedactsEnvironmentToken(t *testing.T) {
	api := &applicationAPI{create: func(context.Context) (int, error) { return 0, errors.New("request with env-very-secret uncertain") }}
	m := vastApplication(t, api)
	advanceVastToCost(t, m)
	m.acceptPrompt(true)
	for m.session != nil {
		nextApplicationEvent(t, m)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m.requestSetup()
	if api.creates.Load() != 1 || !m.creating || m.session != nil || strings.Contains(m.View(), "env-very-secret") {
		t.Fatalf("unsafe uncertain state: %s", m.View())
	}
	if !strings.Contains(m.View(), "checked in Vast") {
		t.Fatalf("missing uncertainty guidance: %s", m.View())
	}
}

func TestApplicationBackCancelsSearchBeforeReplayAndRejectsStaleEvents(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	api := &applicationAPI{search: func(ctx context.Context) ([]vast.Offer, error) {
		close(started)
		<-ctx.Done()
		close(cancelled)
		<-release
		return nil, ctx.Err()
	}}
	m := vastApplication(t, api)
	nextApplicationPrompt(t, m, promptProvider)
	m.acceptPrompt("vast")
	nextApplicationPrompt(t, m, promptWorkload)
	m.acceptPrompt(testRecipe().ID)
	nextApplicationPrompt(t, m, promptIdentity)
	m.acceptPrompt("/fixture/key")
	nextApplicationPrompt(t, m, promptBrowser)
	browser := m.child.(*offerBrowserModel)
	searchDone := make(chan tea.Msg, 1)
	search := m.tagChild(browser.refresh())
	go func() { searchDone <- search() }()
	<-started
	old := m.session.generation
	m.goBack()
	<-cancelled
	if m.session.generation != old {
		t.Fatal("replay overlapped old search")
	}
	close(release)
	m.Update(<-searchDone)
	nextApplicationPrompt(t, m, promptIdentity)
	m.Update(applicationEvent{old, promptRequest{kind: promptToken}})
	m.Update(applicationEvent{old, applicationDone{errors.New("stale")}})
	m.Update(applicationChildMsg{m.childGeneration - 1, catalogui.PickerResultMsg{Value: "stale"}})
	if m.pending.kind != promptIdentity || api.creates.Load() != 0 {
		t.Fatal("late event replaced current prompt")
	}
}

func TestApplicationCustomModelUsesExistingInspectionAndExplicitOffer(t *testing.T) {
	api := &applicationAPI{}
	m := vastApplication(t, api)
	inspections := 0
	m.deps.Setup.InspectModel = func(_ context.Context, repo string) (huggingface.Model, error) {
		inspections++
		return huggingface.Model{Repository: repo, Revision: strings.Repeat("a", 40), Classification: catalog.Result{Status: catalog.Supported, Kind: "text-generation", Engine: "sglang"}}, nil
	}
	// Restart before any answer so the worker receives the injected read-only service.
	m.cleanup()
	m.session = nil
	m.beginSetup()
	nextApplicationPrompt(t, m, promptProvider)
	m.acceptPrompt("vast")
	nextApplicationPrompt(t, m, promptWorkload)
	m.acceptPrompt(customHuggingFaceWorkload)
	nextApplicationPrompt(t, m, promptQuery)
	m.acceptPrompt("owner/model")
	nextApplicationPrompt(t, m, promptHardware)
	m.acceptPrompt(CustomHardware{MinimumVRAMGB: 48, MinimumDiskGB: 100})
	nextApplicationPrompt(t, m, promptCustom)
	m.acceptPrompt(true)
	nextApplicationPrompt(t, m, promptIdentity)
	m.acceptPrompt("/fixture/key")
	nextApplicationPrompt(t, m, promptBrowser)
	if inspections != 1 || api.creates.Load() != 0 {
		t.Fatal("custom flow skipped inspection or prematurely created")
	}
	chooseApplicationOffer(t, m)
	nextApplicationPrompt(t, m, promptCost)
	m.acceptPrompt(false)
	for m.session != nil {
		nextApplicationEvent(t, m)
	}
	if api.creates.Load() != 0 {
		t.Fatal("declining custom cost created an instance")
	}
}

func TestApplicationBackFromCustomReviewRetainsAcceptedHardware(t *testing.T) {
	m := vastApplication(t, &applicationAPI{})
	m.deps.Setup.InspectModel = func(_ context.Context, repo string) (huggingface.Model, error) {
		return huggingface.Model{Repository: repo, Revision: strings.Repeat("a", 40), Classification: catalog.Result{Status: catalog.Supported, Kind: "text-generation", Engine: "sglang"}}, nil
	}
	m.cleanup()
	m.session = nil
	m.beginSetup()
	nextApplicationPrompt(t, m, promptProvider)
	m.acceptPrompt("vast")
	nextApplicationPrompt(t, m, promptWorkload)
	m.acceptPrompt(customHuggingFaceWorkload)
	nextApplicationPrompt(t, m, promptQuery)
	m.acceptPrompt("owner/model")
	nextApplicationPrompt(t, m, promptHardware)
	m.acceptPrompt(CustomHardware{MinimumVRAMGB: 96, MinimumDiskGB: 240})
	nextApplicationPrompt(t, m, promptCustom)

	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	nextApplicationPrompt(t, m, promptHardware)
	m.Update(tea.WindowSizeMsg{Width: 108, Height: 30})
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "96") || !strings.Contains(view, "240") {
		t.Fatalf("returning from review lost accepted hardware values: %s", view)
	}
	answer, err := m.editorAnswer()
	if err != nil || answer != (CustomHardware{MinimumVRAMGB: 96, MinimumDiskGB: 240}) {
		t.Fatalf("unchanged hardware submission = %+v, %v; want 96 GB VRAM and 240 GB disk", answer, err)
	}
	m.acceptPrompt(answer)
	nextApplicationPrompt(t, m, promptCustom)
	if hardware := m.pending.data.(customPrompt).hardware; hardware != (CustomHardware{MinimumVRAMGB: 96, MinimumDiskGB: 240}) {
		t.Fatalf("review hardware = %+v; want 96 GB VRAM and 240 GB disk", hardware)
	}
}

func TestApplicationFrameBoundsAndHiddenSubmission(t *testing.T) {
	for _, size := range [][2]int{{30, 10}, {72, 24}, {108, 30}, {134, 30}, {150, 34}, {320, 80}} {
		m := vastApplication(t, &applicationAPI{})
		advanceVastToCost(t, m)
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		lines := strings.Split(strings.TrimSuffix(m.View(), "\n"), "\n")
		if len(lines) > size[1] {
			t.Fatalf("height overflow %v: %d", size, len(lines))
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("width overflow %v: %q", size, line)
			}
		}
		if !strings.Contains(m.View(), "Enter") {
			t.Fatalf("hidden controls at %v", size)
		}
		m.Update(tea.WindowSizeMsg{Width: 29, Height: 9})
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if m.pending == nil || m.locked {
			t.Fatal("undersized terminal submitted hidden prompt")
		}
	}
}

func TestApplicationChangingRecipeInvalidatesDependentDrafts(t *testing.T) {
	m := vastApplication(t, &applicationAPI{})
	nextApplicationPrompt(t, m, promptProvider)
	m.acceptPrompt("vast")
	nextApplicationPrompt(t, m, promptWorkload)
	m.acceptPrompt(customHuggingFaceWorkload)
	nextApplicationPrompt(t, m, promptQuery)
	m.editor.text = "old/model"
	m.goBack()
	nextApplicationPrompt(t, m, promptWorkload)
	m.acceptPrompt(testRecipe().ID)
	nextApplicationPrompt(t, m, promptIdentity)
	m.goBack()
	nextApplicationPrompt(t, m, promptWorkload)
	m.acceptPrompt(customHuggingFaceWorkload)
	nextApplicationPrompt(t, m, promptQuery)
	if m.editor.text != "" {
		t.Fatalf("changed recipe retained dependent model: %q", m.editor.text)
	}
}

func TestApplicationLongPaidErrorKeepsWarningVisibleAtMinimum(t *testing.T) {
	api := &applicationAPI{create: func(context.Context) (int, error) { return 0, errors.New(strings.Repeat("remote explanation ", 100)) }}
	m := vastApplication(t, api)
	advanceVastToCost(t, m)
	m.acceptPrompt(true)
	for m.session != nil {
		nextApplicationEvent(t, m)
	}
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	if !strings.Contains(m.View(), "Creation must be checked") {
		t.Fatal("long error hid paid warning")
	}
}
