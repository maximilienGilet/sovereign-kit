package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
	builtinrecipes "github.com/maximilienGilet/sovereign-kit/recipes"
)

func rentalReviewFixture() setup.OfferView {
	return setup.OfferView{
		Offer: vast.Offer{
			ID: 42, MachineID: 88, GPUName: "RTX PRO 6000 S", GPUCount: 1, GPUVRAMGB: 97.9, TotalGPUVRAMGB: 97.9,
			CPUCores: 48, CPURAMGB: 257.75, DiskSpaceGB: 1251.32500000000005, InetDownMBps: 7494.8, InetUpMBps: 5233,
			DriverVersion: "570.86.15", HourlyUSD: 1.5037, Location: "Maryland, US", Reliability: 0.999,
		},
		Recipe: recipe.Recipe{
			Name: "Qwen Studio", Kind: "text-generation",
			Profile:      recipe.Profile{Status: "reference", Summary: "Long-context private Qwen route", Evidence: "Historical synthetic evidence"},
			Runtime:      recipe.Runtime{Engine: "sglang", Image: "lmsysorg/sglang@sha256:e21dd539b36ea7842101393ec3fe3b0d453626cd8251e4d4db33af0cd97d7f0b"},
			Model:        recipe.Model{Repository: "RadixArk/Qwen3.8-27B-NVFP4", Revision: "319f741cce68d7914884900c138a1fbb70a42f30"},
			Serve:        recipe.Serve{ContextWindow: 262144, MaxOutputTokens: 16384, MaxRunningRequests: 5},
			Requirements: recipe.Requirements{GPUModel: "RTX PRO 6000 S", GPUCount: 1, StrictGPU: true, MinimumVRAMGB: 96, MinimumDiskGB: 120},
		},
		MonthlyUSD: 1097.70,
		AnnualUSD:  13172.41,
	}
}

func TestRentalReviewUsesOnlyAuditableRatios(t *testing.T) {
	review := newRentalReview(rentalReviewFixture(), 120)
	if !review.skuMatch || !review.countMatch {
		t.Fatalf("expected exact strict matches: %#v", review)
	}
	if review.vramRatio < 1.019 || review.vramRatio > 1.021 {
		t.Fatalf("VRAM ratio = %f, want offered/required ratio", review.vramRatio)
	}
	if review.diskRatio < 10.427 || review.diskRatio > 10.429 {
		t.Fatalf("disk ratio = %f, want available/requested ratio", review.diskRatio)
	}

}

func TestRentalReviewSummaryIsScannableAndExact(t *testing.T) {
	summary := newRentalReview(rentalReviewFixture(), 120).accessibleSummary()
	for _, want := range []string{
		"Qwen Studio", "REFERENCE", "Historical synthetic evidence",
		"Offer 42", "Machine 88", "1× RTX PRO 6000 S", "97.9 GB per GPU", "97.9 GB total VRAM",
		"48 CPU cores", "257.8 GB RAM", "120 GB allocated", "1,251.3 GB available",
		"7,494.8 MB/s down", "5,233 MB/s up", "Maryland, US", "99.9%",
		"$1.5037/hour", "$1,097.70/month", "RadixArk/Qwen3.8-27B-NVFP4", "319f741cce68d7914884900c138a1fbb70a42f30",
		"262,144 context", "16,384 output", "5 concurrent", "billing starts immediately",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
	for _, unwanted := range []string{" | ", "1251.32500000000005"} {
		if strings.Contains(summary, unwanted) {
			t.Fatalf("summary contains %q:\n%s", unwanted, summary)
		}
	}
}

func TestRentalReviewRendersWideAndNarrowCockpits(t *testing.T) {
	for _, test := range []struct {
		name   string
		width  int
		height int
	}{
		{name: "wide", width: 150, height: 30},
		{name: "narrow", width: 80, height: 24},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := newRentalReviewModel(rentalReviewFixture(), 120)
			model.width = test.width
			model.height = test.height
			view := model.View()
			for _, want := range []string{"SOVEREIGN KIT", "DEPLOYMENT REVIEW", "VRAM", "Disk", "$1.50", "Create paid instance", "Cancel"} {
				if !strings.Contains(view, want) {
					t.Fatalf("%s cockpit missing %q:\n%s", test.name, want, view)
				}
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > test.width {
					t.Fatalf("%s cockpit line width %d exceeds %d: %q", test.name, lipgloss.Width(line), test.width, line)
				}
			}
			if lipgloss.Height(view) > test.height {
				t.Fatalf("%s cockpit height %d exceeds %d", test.name, lipgloss.Height(view), test.height)
			}
		})
	}
}

func TestRentalReviewDefaultsToCancel(t *testing.T) {
	model := newRentalReviewModel(rentalReviewFixture(), 120)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	cancelled := updated.(rentalReviewModel)
	if !cancelled.done || cancelled.confirmed {
		t.Fatalf("default enter must cancel: %#v", cancelled)
	}

	model = newRentalReviewModel(rentalReviewFixture(), 120)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	model = updated.(rentalReviewModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	confirmed := updated.(rentalReviewModel)
	if !confirmed.done || !confirmed.confirmed {
		t.Fatalf("explicit create selection was not confirmed: %#v", confirmed)
	}
}

func TestRentalReviewShowsCapacityImmediatelyWithoutVerificationAnimation(t *testing.T) {
	model := newRentalReviewModel(rentalReviewFixture(), 120)
	view := model.View()
	for _, want := range []string{"97.9 GB / GPU", "96 GB required", "120 GB required", "│", "Cancel"} {
		if !strings.Contains(view, want) {
			t.Fatalf("review missing %q:\n%s", want, view)
		}
	}
	if model.Init() != nil || strings.Contains(view, "RADAR") || strings.Contains(view, "VERIFYING") {
		t.Fatal("review simulates verification")
	}
}

func TestAccessibleRentalReviewUsesInjectedStreams(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("ACCESSIBLE", "")
	for _, test := range []struct {
		name      string
		input     string
		confirmed bool
	}{
		{name: "explicit yes", input: "yes\n", confirmed: true},
		{name: "default no", input: "\n", confirmed: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			prompter := NewHuhPrompter(strings.NewReader(test.input), &output)
			confirmed, err := runRentalReview(context.Background(), prompter, rentalReviewFixture(), 120)
			if err != nil {
				t.Fatal(err)
			}
			if confirmed != test.confirmed {
				t.Fatalf("confirmed = %t, want %t", confirmed, test.confirmed)
			}
			rendered := output.String()
			for _, want := range []string{"Deployment review", "Qwen Studio", "billing starts immediately", "Create this paid Vast instance? [y/N]"} {
				if !strings.Contains(rendered, want) {
					t.Fatalf("accessible output missing %q:\n%s", want, rendered)
				}
			}
		})
	}
}

func TestRentalReviewShowsRuntimeOptimizations(t *testing.T) {
	view := rentalReviewFixture()
	view.Recipe.Runtime = recipe.Runtime{
		Engine:                 "vllm",
		Image:                  "ghcr.io/maximiliengilet/sovereign-kit-vllm@sha256:runtime",
		Quantization:           "modelopt",
		KVCacheDType:           "turboquant_4bit_nc",
		GPUMemoryUtilization:   0.94,
		DisableAsyncScheduling: true,
	}
	view.Recipe.Speculative = &recipe.Speculative{Algorithm: "qwen3_5_mtp", NumDraftTokens: 4}
	summary := newRentalReview(view, 120).accessibleSummary()
	for _, want := range []string{"vllm · modelopt · turboquant_4bit_nc · MTP ×4", "GPU memory: 94% · async scheduling disabled"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "qwen3_5") {
		t.Fatalf("summary exposes backend architecture name as a model version:\n%s", summary)
	}
}

func TestBundledRecipesFitSupportedTerminalSizes(t *testing.T) {
	bundled, err := builtinrecipes.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	for _, selectedRecipe := range bundled {
		view := rentalReviewFixture()
		view.Recipe = selectedRecipe
		for _, size := range []struct {
			width  int
			height int
		}{
			{width: 40, height: 16},
			{width: 64, height: 20},
			{width: 80, height: 24},
			{width: 150, height: 30},
			{width: 107, height: 22},
			{width: 108, height: 22},
			{width: 108, height: 30},
			{width: 133, height: 30},
			{width: 134, height: 30},
		} {
			model := newRentalReviewModel(view, selectedRecipe.Requirements.MinimumDiskGB)
			model.width = size.width
			model.height = size.height
			rendered := model.View()
			assertRentalReviewFits(t, rendered, size.width, size.height)
			if !strings.Contains(rendered, selectedRecipe.Name) {
				t.Fatalf("%s at %dx%d omitted recipe name:\n%s", selectedRecipe.ID, size.width, size.height, rendered)
			}
			if size.width >= 134 && size.height >= 30 {
				for _, expected := range []string{selectedRecipe.Model.Repository, selectedRecipe.Runtime.Engine} {
					if !strings.Contains(rendered, expected) {
						t.Fatalf("%s wide view omitted %q:\n%s", selectedRecipe.ID, expected, rendered)
					}
				}
			}
		}
	}
}

func assertRentalReviewFits(t *testing.T, view string, width, height int) {
	t.Helper()
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > width {
			t.Fatalf("line width %d exceeds %d: %q", lipgloss.Width(line), width, line)
		}
	}
	if lipgloss.Height(view) > height {
		t.Fatalf("height %d exceeds %d:\n%s", lipgloss.Height(view), height, view)
	}
}

type rentalPromptWriter struct {
	mu       sync.Mutex
	output   bytes.Buffer
	prompted chan struct{}
	once     sync.Once
}

func (writer *rentalPromptWriter) Write(value []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	written, err := writer.output.Write(value)
	if strings.Contains(writer.output.String(), "Create this paid Vast instance? [y/N]") {
		writer.once.Do(func() { close(writer.prompted) })
	}
	return written, err
}

func TestAccessibleRentalReviewCancelsBlockedInput(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("ACCESSIBLE", "")
	input, blockedInput := io.Pipe()
	defer input.Close()
	defer blockedInput.Close()
	output := &rentalPromptWriter{prompted: make(chan struct{})}
	prompter := NewHuhPrompter(input, output)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := runRentalReview(ctx, prompter, rentalReviewFixture(), 120)
		result <- err
	}()
	select {
	case <-output.prompted:
	case <-time.After(time.Second):
		t.Fatal("accessible confirmation prompt did not appear")
	}
	cancel()
	select {
	case err := <-result:
		if err != context.Canceled {
			t.Fatalf("error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("accessible confirmation ignored context cancellation")
	}
}
