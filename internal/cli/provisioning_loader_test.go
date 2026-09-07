package cli

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/huggingface"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/muesli/termenv"
	"strings"
	"testing"
	"time"
)

// Losing the active operation below a wrapped billing warning is unsafe on small terminals.
func TestProvisioningCompactKeepsOperationAndWarning(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	m.screen = "working"
	m.applyProgress(setup.Progress{Stage: setup.ProgressWaiting, InstanceID: 12345})
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	view := ansi.Strip(m.View())
	for _, want := range []string{"Waiting", "12345", "billing", "Ctrl+C"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q in %s", want, view)
		}
	}
	if strings.Contains(view, "LIVE") {
		t.Fatal("waiting is not a verified route")
	}
}

type progressWorkloadPrompter struct {
	fakeSetupPrompter
	stages []string
}

func (p *progressWorkloadPrompter) SetupProgress(progress setup.Progress) {
	p.stages = append(p.stages, progress.Stage)
}
func TestHuggingFaceNamesSearchAndInspectionBeforeCalls(t *testing.T) {
	p := &progressWorkloadPrompter{fakeSetupPrompter: fakeSetupPrompter{workload: customHuggingFaceWorkload, hfQuery: "model", hfModel: "owner/model"}}
	resolveVastWorkload(context.Background(), p, []recipe.Recipe{testRecipe()}, func(context.Context, string, int) ([]huggingface.SearchResult, error) {
		if len(p.stages) != 1 || p.stages[0] != "model-search" {
			t.Errorf("search operation unnamed: %v", p.stages)
		}
		return []huggingface.SearchResult{{Repository: "owner/model"}}, nil
	}, func(context.Context, string) (huggingface.Model, error) {
		if len(p.stages) != 2 || p.stages[1] != "model-inspect" {
			t.Errorf("inspection operation unnamed: %v", p.stages)
		}
		return huggingface.Model{}, nil
	})
}

func TestProvisioningFramesAndStaleTimers(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	m.screen = "working"
	m.applyProgress(setup.Progress{Stage: setup.ProgressCreating})
	m.Update(tea.WindowSizeMsg{Width: 108, Height: 30})
	m.applyProgress(setup.Progress{Stage: setup.ProgressWaiting, InstanceID: 123})
	m.Update(tea.WindowSizeMsg{Width: 108, Height: 30})
	tick := provisioningTick{m.generation, m.loader.epoch, m.loader.started.Add(250 * time.Millisecond)}
	_, cmd := m.Update(tick)
	if cmd == nil {
		t.Fatal("active timer stopped")
	}
	first := m.View()
	tick.at = tick.at.Add(1400 * time.Millisecond)
	m.Update(tick)
	second := m.View()
	if first == second {
		t.Fatal("Magnetic Pulse lighting did not animate")
	}
	if !strings.Contains(ansi.Strip(second), "✓ Instance created") {
		t.Fatal("actual completed step missing")
	}
	for _, frame := range []string{first, second} {
		for _, line := range strings.Split(frame, "\n") {
			if ansi.StringWidth(line) > 108 {
				t.Fatal("unstable frame width")
			}
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	_, cmd = m.Update(tick)
	if cmd != nil {
		t.Fatal("exit confirmation continued timer")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_, cmd = m.Update(tick)
	if cmd != nil {
		t.Fatal("stale epoch restarted timer")
	}
	m.screen = "prompt"
	m.Update(tea.WindowSizeMsg{Width: 108, Height: 30})
	_, cmd = m.Update(tick)
	if cmd != nil {
		t.Fatal("prompt continued timer")
	}
}

func TestMagneticPulsePhaseSurvivesStageChanges(t *testing.T) {
	at := time.Unix(100, 0)
	l := provisioningLoader{}
	l.stage("Waiting", at)
	l.now = at.Add(720 * time.Millisecond)
	before := l.forgeElapsed()
	l.stage("Securing SSH", l.now)
	after := l.forgeElapsed()
	if before != 720*time.Millisecond || after != before || l.now.Sub(l.started) != 0 {
		t.Fatalf("stage reset Magnetic Pulse phase: before=%s after=%s operation=%s", before, after, l.now.Sub(l.started))
	}
}

func TestProvisioningOnlyActualForwardMilestonesComplete(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	m.applyProgress(setup.Progress{Stage: setup.ProgressWaiting})
	m.applyProgress(setup.Progress{Stage: setup.ProgressCreating})
	if len(m.loader.completed) != 0 {
		t.Fatal("backward progress manufactured completion")
	}
	m.applyProgress(setup.Progress{Stage: setup.ProgressCreated})
	m.applyProgress(setup.Progress{Stage: setup.ProgressWaiting})
	m.applyProgress(setup.Progress{Stage: setup.ProgressHostKeys})
	m.screen = "error"
	if len(m.loader.completed) != 2 {
		t.Fatalf("unexpected completed milestones: %v", m.loader.completed)
	}
	for _, done := range m.loader.completed {
		if strings.Contains(done, "SSH host secured") {
			t.Fatal("SSH host marked secured before trust persistence")
		}
	}
}

func TestHuggingFaceUsesCompactLoaderAtLargeSize(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	m.screen = "working"
	m.applyProgress(setup.Progress{Stage: setup.ProgressModelInspect})
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 34})
	view := ansi.Strip(m.View())
	if strings.Contains(view, "◇") || !strings.Contains(view, "Inspecting Hugging Face") {
		t.Fatal("model inspection should use compact waiting")
	}
}

func TestExitConfirmationSuspendsChildAnimationWithoutLosingSearch(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	m.openPrompt(promptRequest{kind: promptBrowser, data: offerBrowserPrompt{ctx: context.Background(), recipe: testRecipe(), search: func(context.Context, setup.OfferQuery) (setup.OfferSearchResult, error) { return browserFixture(), nil }}})
	b := m.child.(*offerBrowserModel)
	b.loading = false
	b.frame = 0
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	_, cmd := m.Update(applicationChildMsg{m.childGeneration, browserTickMsg{b.animation}})
	if cmd != nil || b.frame != 0 {
		t.Fatal("confirmation continued child animation")
	}
	b.loading = true
	_, cmd = m.Update(applicationChildMsg{m.childGeneration, browserSearchMsg{generation: b.generation, result: browserFixture()}})
	if cmd != nil || b.loading || len(b.views) == 0 {
		t.Fatal("confirmation lost search result or restarted animation")
	}
}

func TestProvisioningFutureStagesOnlyOnRoomyWorkingScreen(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	m.screen = "working"
	m.applyProgress(setup.Progress{Stage: setup.ProgressWaiting, InstanceID: 42})
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 34})
	view := ansi.Strip(m.View())
	for _, want := range []string{"○ Secure SSH host", "○ Launch inference server", "○ Save configuration"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing future stage %s: %s", want, view)
		}
	}
	if strings.Count(view, "Waiting for the instance") != 1 {
		t.Fatal("active label duplicated")
	}
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	view = ansi.Strip(m.View())
	if strings.Contains(view, "○") || !strings.Contains(view, "#42") || !strings.Contains(view, "billing") {
		t.Fatal("future stages displaced essential compact content")
	}
}

func TestProvisioningShimmerFixedWidthAndPlainOutput(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	a, b := shimmer("Waiting for instance", time.Second), shimmer("Waiting for instance", 2*time.Second)
	if a == b || ansi.Strip(a) != "Waiting for instance" || ansi.StringWidth(a) != ansi.StringWidth(b) {
		t.Fatal("shimmer changed text/width or did not move")
	}
	l := provisioningLoader{label: "Waiting", started: time.Unix(0, 0), now: time.Unix(2, 0), completed: []string{"Created"}}
	plain := l.view(108, 26, false, "")
	if strings.Contains(plain, "\x1b") || !strings.Contains(plain, "> Waiting") {
		t.Fatal("plain loader not accessible")
	}
}

func TestProvisioningOfferSearchClockStopsAtResultAndExit(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	browser := newOfferBrowser(context.Background(), testRecipe(), nil)
	browser.loading = true
	m.child = browser
	m.screen = "prompt"
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 108, Height: 30})
	if cmd == nil {
		t.Fatal("offer search has no spinner clock")
	}
	tick := provisioningTick{m.generation, m.loader.epoch, time.Now().Add(time.Second)}
	before := m.View()
	m.Update(tick)
	if before == m.View() {
		t.Fatal("search spinner did not move")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	_, cmd = m.Update(tick)
	if cmd != nil {
		t.Fatal("exit confirmation continued search animation")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	browser.loading = false
	m.Update(tea.WindowSizeMsg{Width: 108, Height: 30})
	_, cmd = m.Update(tick)
	if cmd != nil {
		t.Fatal("completed search continued spinner")
	}
}

func TestProvisioningEverySizeReservesPaidIdentity(t *testing.T) {
	for _, size := range [][2]int{{30, 10}, {72, 24}, {108, 30}, {150, 34}, {220, 48}} {
		m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
		m.screen = "working"
		for _, stage := range []string{setup.ProgressCreating, setup.ProgressWaiting, setup.ProgressHostKeys, setup.ProgressLaunching, setup.ProgressSaving} {
			m.applyProgress(setup.Progress{Stage: stage, InstanceID: 456})
		}
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := ansi.Strip(m.View())
		for _, want := range []string{"Saving", "#456", "billing", "Ctrl+C"} {
			if !strings.Contains(view, want) {
				t.Errorf("%v missing %s: %s", size, want, view)
			}
		}
	}
}

func TestProvisioningErrorFreezesAmberMarkerAndSaveHasNoReady(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	m.screen = "working"
	m.applyProgress(setup.Progress{Stage: setup.ProgressWaiting, InstanceID: 42})
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 34})
	tick := provisioningTick{m.generation, m.loader.epoch, time.Now().Add(time.Second)}
	m.Update(tick)
	m.applyProgress(setup.Progress{Stage: setup.ProgressLaunching, InstanceID: 42})
	m.applyServerLogs(setup.ServerLogs{Text: `SOVKIT_DOWNLOAD {"current":600,"total":1000}`, CheckedAt: time.Now()})
	m.screen = "error"
	m.errText = "provider failed"
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 34})
	first := m.View()
	if !strings.Contains(ansi.Strip(first), "··") || forgeBuiltCellCount(first) < 20 {
		t.Fatal("error missing frozen partial crystal")
	}
	tick.at = tick.at.Add(time.Second)
	_, cmd := m.Update(tick)
	if cmd != nil || m.View() != first {
		t.Fatal("error motif animated")
	}
	m.screen = "saved"
	if strings.Contains(ansi.Strip(m.View()), "✓") || strings.Contains(m.View(), "LIVE") {
		t.Fatal("save manufactured verified success")
	}
}

func TestProvisioningErrorBeforeInstanceIDFreezesForgeIntake(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	m.screen = "working"
	m.applyProgress(setup.Progress{Stage: setup.ProgressCreating})
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 34})
	m.screen = "error"
	m.errText = "creation failed before Vast returned an ID"
	view := ansi.Strip(m.View())
	crystal := false
	for _, r := range view {
		if r >= 0x2800 && r <= 0x28ff {
			crystal = true
			break
		}
	}
	if !crystal || forgeBuiltCellCount(view) < 5 {
		t.Fatalf("pre-instance error lost frozen Forge Intake: %s", view)
	}
}

func TestProvisioningRootStartsAnimationOnlyWhileWorking(t *testing.T) {
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	m.screen = "working"
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 108, Height: 30})
	if cmd == nil {
		t.Fatal("working screen did not start animation")
	}
	m.screen = "error"
	_, cmd = m.Update(tea.WindowSizeMsg{Width: 108, Height: 30})
	if cmd != nil {
		t.Fatal("error screen scheduled animation")
	}
}
