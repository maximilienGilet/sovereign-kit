package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

func TestApplicationOfferBrowserOwnsFilterKeysBackAndReview(t *testing.T) {
	api := &applicationAPI{}
	m := vastApplication(t, api)
	nextApplicationPrompt(t, m, promptProvider)
	m.acceptPrompt("vast")
	nextApplicationPrompt(t, m, promptWorkload)
	m.acceptPrompt(testRecipe().ID)
	nextApplicationPrompt(t, m, promptIdentity)
	m.acceptPrompt("/fixture/key")
	nextApplicationPrompt(t, m, promptBrowser)
	browser := m.child.(*offerBrowserModel)
	m.Update(m.tagChild(browser.refresh())())
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if m.exitConfirm || browser.filter.Value() != "q" {
		t.Fatal("root intercepted filter input")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.pending.kind != promptBrowser || browser.filtering {
		t.Fatal("filter Esc navigated root")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.pending.kind != promptBrowser || browser.details || api.creates.Load() != 0 {
		t.Fatal("inspection mutated setup")
	}
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 34})
	_, selectCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(selectCmd())
	nextApplicationPrompt(t, m, promptCost)
	if m.child.(embeddedRentalReview).model.createSelected || api.creates.Load() != 0 {
		t.Fatal("offer selection approved creation")
	}
	oldGeneration := m.childGeneration
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	nextApplicationPrompt(t, m, promptBrowser)
	if m.child == browser || m.locked {
		t.Fatal("Back reused old browser")
	}
	m.Update(applicationChildMsg{oldGeneration, browserSelectedMsg{offer: browserFixture().Views[0].Offer}})
	if m.pending.kind != promptBrowser || api.creates.Load() != 0 {
		t.Fatal("stale selection changed current screen")
	}
}

func TestApplicationOfferBrowserBackRetainsQueryButRefreshesFacts(t *testing.T) {
	searches := 0
	api := &applicationAPI{search: func(context.Context) ([]vast.Offer, error) {
		searches++
		return []vast.Offer{{ID: 42, GPUName: "test", GPUCount: 1, GPUVRAMGB: 200, DiskSpaceGB: 1000, HourlyUSD: float64(searches), Location: "Paris, FR"}}, nil
	}}
	m := vastApplication(t, api)
	nextApplicationPrompt(t, m, promptProvider)
	m.acceptPrompt("vast")
	nextApplicationPrompt(t, m, promptWorkload)
	m.acceptPrompt(testRecipe().ID)
	nextApplicationPrompt(t, m, promptIdentity)
	m.acceptPrompt("/fixture/key")
	nextApplicationPrompt(t, m, promptBrowser)
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 34})
	key := func(key string) tea.Cmd {
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		return cmd
	}
	key("f")
	key("France")
	key(" ")
	_, apply := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updateApplicationCommand(t, m, apply)
	updateApplicationCommand(t, m, key("s"))
	// Editing and canceling a draft must not alter the query retained on Back.
	key("f")
	key("Japan")
	key(" ")
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	old := m.child.(*offerBrowserModel)
	_, choose := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updateApplicationCommand(t, m, choose)
	nextApplicationPrompt(t, m, promptCost)
	oldPrice := m.pending.data.(costPrompt).view.Offer.HourlyUSD
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	nextApplicationPrompt(t, m, promptBrowser)
	fresh := m.child.(*offerBrowserModel)
	if !reflect.DeepEqual(fresh.query.Countries, []string{"FR"}) || fresh.query.Sort != vast.SortReliability {
		t.Fatalf("Back lost applied query: %+v", fresh.query)
	}
	if fresh == old || len(fresh.views) != 0 || fresh.selectedID != 0 || !fresh.loading {
		t.Fatal("Back reused offer selection or facts")
	}
	// Existing worker helpers leave Init commands unexecuted; run the fresh query.
	m.Update(m.tagChild(fresh.refresh())())
	_, choose = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updateApplicationCommand(t, m, choose)
	nextApplicationPrompt(t, m, promptCost)
	if price := m.pending.data.(costPrompt).view.Offer.HourlyUSD; price <= oldPrice || api.creates.Load() != 0 {
		t.Fatalf("review did not use fresh facts: old=%v new=%v", oldPrice, price)
	}
}

func TestApplicationOfferBrowserRestoresCopiedQueryBeforeInit(t *testing.T) {
	for _, change := range []struct {
		name      string
		kind      promptKind
		old, next any
	}{
		{name: "unchanged"},
		{name: "provider", kind: promptProvider, old: "vast", next: "manual"},
		{name: "recipe", kind: promptWorkload, old: "old-recipe", next: "new-recipe"},
		{name: "model", kind: promptModel, old: "old/model", next: "new/model"},
		{name: "hardware", kind: promptHardware, old: CustomHardware{MinimumVRAMGB: 48, MinimumDiskGB: 100}, next: CustomHardware{MinimumVRAMGB: 96, MinimumDiskGB: 200}},
	} {
		t.Run(change.name, func(t *testing.T) {
			m := newApplication(context.Background(), t.TempDir()+"/config.toml", "fixture", "home", ApplicationDependencies{})
			var queries []setup.OfferQuery
			request := promptRequest{kind: promptBrowser, data: offerBrowserPrompt{ctx: context.Background(), recipe: testRecipe(), search: func(_ context.Context, q setup.OfferQuery) (setup.OfferSearchResult, error) {
				queries = append(queries, q)
				return browserFixture(), nil
			}}, reply: make(chan any, 1)}
			m.openPrompt(request)
			old := m.child.(*offerBrowserModel)
			old.query = setup.OfferQuery{Countries: []string{"FR"}, Sort: vast.SortReliability}
			m.closeBrowser()
			old.query.Countries[0] = "JP" // The old child cannot mutate the saved query.
			if change.kind != "" {
				m.drafts[change.kind] = change.old
				m.invalidateDependentDrafts(change.kind, change.next)
			}
			cmd := m.openPrompt(request)
			updateApplicationCommand(t, m, cmd) // Execute the actual initial search, not a later refresh.
			defer m.closeBrowser()
			want := setup.OfferQuery{Countries: []string{"FR"}, Sort: vast.SortReliability}
			if change.kind != "" {
				want = setup.OfferQuery{Sort: vast.SortPrice}
			}
			if len(queries) != 1 || !reflect.DeepEqual(queries[0], want) {
				t.Fatalf("initial query=%+v want %+v", queries, want)
			}
			if change.kind == "" {
				m.child.(*offerBrowserModel).query.Countries[0] = "DE"
				// Reopening without committing must also leave the retained copy alone.
				m.child.(*offerBrowserModel).Close()
				cmd = m.openPrompt(request)
				updateApplicationCommand(t, m, cmd)
				if !reflect.DeepEqual(queries[1], want) {
					t.Fatalf("new child aliased retained query: %+v", queries[1])
				}
			}
		})
	}
}

func TestOfferBrowserRootFrames(t *testing.T) {
	for _, size := range [][2]int{{30, 10}, {72, 24}, {108, 30}, {150, 34}, {320, 80}} {
		for _, state := range []string{"list", "filter", "details"} {
			m := newApplication(context.Background(), t.TempDir()+"/config.toml", "fixture", "home", ApplicationDependencies{})
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			cmd := m.openPrompt(promptRequest{kind: promptBrowser, data: offerBrowserPrompt{ctx: context.Background(), recipe: testRecipe(), search: func(context.Context, setup.OfferQuery) (setup.OfferSearchResult, error) { return browserFixture(), nil }}, reply: make(chan any, 1)})
			updateApplicationCommand(t, m, cmd)
			browser := m.child.(*offerBrowserModel)
			for i := 0; i < 4; i++ {
				m.Update(applicationChildMsg{m.childGeneration, browserTickMsg{browser.animation}})
			}
			if state == "filter" {
				m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
			}
			if state == "details" {
				m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
			}
			view := ansi.Strip(m.View())
			lines := strings.Split(strings.TrimSuffix(view, "\n"), "\n")
			if len(lines) > size[1] {
				t.Fatalf("%v %s height=%d", size, state, len(lines))
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > size[0] {
					t.Fatalf("%v %s overflow %q", size, state, line)
				}
			}
			if state == "list" && !strings.Contains(view, "> 1× RTX PRO") {
				t.Fatalf("selected row hidden: %s", view)
			}
			if state == "filter" && !strings.Contains(view, "> [x] All countries") {
				t.Fatalf("focused country hidden: %s", view)
			}
			if !strings.Contains(view, "Esc") {
				t.Fatalf("actions hidden: %s", view)
			}
			if state == "details" {
				for i := 0; i < 30; i++ {
					m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
				}
				if !strings.Contains(ansi.Strip(m.View()), "billing starts immediately") && size[0] > 30 {
					t.Fatalf("inspection bottom unreachable: %s", m.View())
				}
			}
			if os.Getenv("SOVKIT_CAPTURE_OFFERS") != "" {
				t.Logf("FIXTURE ROOT %dx%d %s\n%s", size[0], size[1], state, view)
				path := filepath.Join(os.Getenv("SOVKIT_CAPTURE_OFFERS"), fmt.Sprintf("fixture-root-%dx%d-%s.txt", size[0], size[1], state))
				if err := os.WriteFile(path, []byte(view), 0600); err != nil {
					t.Fatal(err)
				}
			}
			m.closeBrowser()
		}
	}
}

func TestAccessibleOfferBrowserFiltersSortsInspectsAndSelects(t *testing.T) {
	var queries []setup.OfferQuery
	search := func(_ context.Context, q setup.OfferQuery) (setup.OfferSearchResult, error) {
		queries = append(queries, q)
		return browserFixture(), nil
	}
	var output strings.Builder
	p := NewAccessiblePrompter(strings.NewReader("f\nFR,DE\ns\ni\n1\nr\n2\n"), &output)
	selected, err := p.BrowseOffers(context.Background(), testRecipe(), search)
	if err != nil || selected.ID != 43 || len(queries) != 4 || len(queries[1].Countries) != 2 {
		t.Fatalf("selection=%+v queries=%+v err=%v", selected, queries, err)
	}
	if !strings.Contains(output.String(), "Machine") || strings.Contains(output.String(), "\x1b") {
		t.Fatal("inspection missing or ANSI emitted")
	}
	p = NewAccessiblePrompter(strings.NewReader(""), io.Discard)
	if _, err = p.BrowseOffers(context.Background(), testRecipe(), search); err != io.EOF {
		t.Fatalf("EOF=%v", err)
	}
}

func TestOfferBrowserRootPreservesColumnAlignmentOnLastLine(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config.toml", "fixture", "home", ApplicationDependencies{})
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 34})
	cmd := m.openPrompt(promptRequest{kind: promptBrowser, data: offerBrowserPrompt{ctx: context.Background(), recipe: testRecipe(), search: func(context.Context, setup.OfferQuery) (setup.OfferSearchResult, error) { return browserFixture(), nil }}, reply: make(chan any, 1)})
	updateApplicationCommand(t, m, cmd)
	defer m.closeBrowser()
	var top, bottom int = -1, -1
	for _, line := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(line, "╭") {
			top = strings.Index(line, "╭")
		}
		if strings.Contains(line, "╰") {
			bottom = strings.Index(line, "╰")
		}
	}
	if top != bottom || top < 0 {
		t.Errorf("panel left edge shifts: top=%d bottom=%d", top, bottom)
	}
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	var starts []int
	for _, line := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(line, "[ ]") {
			starts = append(starts, strings.Index(line, "[ ]"))
		}
	}
	for _, column := range starts {
		if column != 2 {
			t.Errorf("country marker moved to column %d", column)
		}
	}
}

func TestAccessibleOfferSearchErrorsRedactActiveKeyBeforeRetry(t *testing.T) {
	var output strings.Builder
	api := &applicationAPI{search: func(context.Context) ([]vast.Offer, error) {
		return nil, errors.New("request reflected private-api-key")
	}}
	err := Setup(context.Background(), strings.NewReader("1\n1\n/fixture/key\nq\n"), &output, t.TempDir()+"/config.toml", "alice", SetupDependencies{
		Recipes: []recipe.Recipe{testRecipe()}, Getenv: func(name string) string {
			if name == "VAST_API_KEY" {
				return "private-api-key"
			}
			return ""
		}, HomeDir: func() (string, error) { return "/fixture", nil },
		RunVast: func(ctx context.Context, token, identity string, w VastWorkload, operator setup.Operator) (setup.Result, error) {
			return setup.RunVast(ctx, token, w.Recipe, setup.Options{OfferLimit: 100}, setup.Dependencies{NewAPI: func(string) setup.VastAPI { return api }, Operator: operator, ValidateIdentity: func(string) error { return nil }})
		},
	})
	if err == nil || strings.Contains(output.String(), "private-api-key") || !strings.Contains(output.String(), "[redacted]") {
		t.Fatalf("unsafe retry diagnostic: %v\n%s", err, output.String())
	}
}

// Explicitly opt-in fixture: every provider/SSH/server edge comes from the
// existing fake applicationAPI/scanner/trust/server, never production wiring.
func TestPreviewOfferBrowserPTY(t *testing.T) {
	if os.Getenv("SOVKIT_PREVIEW_OFFERS") != "1" {
		t.Skip("opt-in fixture PTY only")
	}
	m := vastApplication(t, &applicationAPI{})
	m.cleanup()
	m.session = nil
	_, err := tea.NewProgram(m, tea.WithInput(os.Stdin), tea.WithOutput(os.Stdout), tea.WithAltScreen()).Run()
	if err != nil {
		t.Fatal(err)
	}
}
