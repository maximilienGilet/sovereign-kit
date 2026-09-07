package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/catalog"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/huggingface"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

func TestAccessiblePrompterUsesInjectedStreamsAndNeverPrintsSecret(t *testing.T) {
	var output bytes.Buffer
	p := NewAccessiblePrompter(strings.NewReader("2\nsecret-token\ngpu.example\n\n\n/key\n/known\n"), &output)
	ctx := context.Background()
	provider, err := p.SelectProvider(ctx)
	if err != nil || provider != "manual" {
		t.Fatalf("provider=%q err=%v", provider, err)
	}
	token, err := p.VastAPIKey(ctx)
	if err != nil || token != "secret-token" {
		t.Fatalf("token retrieval failed: %v", err)
	}
	route, err := p.ManualRoute(ctx, "alice")
	if err != nil || route.Host != "gpu.example" || route.Port != 22 || route.User != "alice" || route.IdentityFile != "/key" || route.KnownHostsFile != "/known" {
		t.Fatalf("route=%+v err=%v", route, err)
	}
	if strings.Contains(output.String(), "secret-token") || strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("unsafe output: %q", output.String())
	}
}

func TestAccessiblePrompterEOFNeverApprovesOrReturnsPartialSecret(t *testing.T) {
	p := NewAccessiblePrompter(strings.NewReader("partial-secret"), io.Discard)
	token, err := p.VastAPIKey(context.Background())
	if token != "" || !errors.Is(err, io.EOF) {
		t.Fatalf("token exposed/EOF lost: err=%v", err)
	}
	p = NewAccessiblePrompter(strings.NewReader(""), io.Discard)
	yes, err := p.ConfirmCost(context.Background(), setup.OfferView{}, 1)
	if yes || !errors.Is(err, io.EOF) {
		t.Fatalf("approved=%v err=%v", yes, err)
	}
}

func TestAccessibleServerLogsAnnounceMeasuredDownloadWithoutRawTelemetry(t *testing.T) {
	var output bytes.Buffer
	p := NewAccessiblePrompter(strings.NewReader(""), &output)
	p.ServerLogSnapshot(setup.ServerLogs{Text: `SOVKIT_DOWNLOAD {"current":5368709120,"total":10737418240}`})
	p.ServerLogSnapshot(setup.ServerLogs{Text: `SOVKIT_DOWNLOAD {"current":5368709120,"total":10737418240}`})
	text := output.String()
	if strings.Count(text, "Model download: 5.00 GiB / 10.00 GiB — 50.0%") != 1 {
		t.Fatalf("measured update missing or duplicated: %q", text)
	}
	if strings.Contains(text, "SOVKIT_DOWNLOAD") {
		t.Fatalf("raw telemetry leaked into accessible output: %q", text)
	}
	output.Reset()
	p.ServerLogSnapshot(setup.ServerLogs{Text: `SOVKIT_DOWNLOAD {"current":1024,"total":0}`})
	if strings.Contains(output.String(), "%") || !strings.Contains(output.String(), "1.0 KiB · total unknown") {
		t.Fatalf("unknown total invented progress: %q", output.String())
	}
}

func TestAccessibleChoicesAndCostRequireExplicitConsent(t *testing.T) {
	var output bytes.Buffer
	p := NewAccessiblePrompter(strings.NewReader("9\n1\n\nyes\n"), &output)
	choice, err := p.SelectWorkload(context.Background(), []recipe.Recipe{{ID: "solo", Name: "Solo"}})
	if err != nil || choice != "solo" {
		t.Fatalf("choice=%q err=%v", choice, err)
	}
	view := setup.OfferView{Offer: vast.Offer{ID: 9, GPUName: "RTX 5090", HourlyUSD: 1}, MonthlyUSD: 730, AnnualUSD: 8760}
	yes, err := p.ConfirmCost(context.Background(), view, 100)
	if err != nil || yes {
		t.Fatalf("default approval=%v err=%v", yes, err)
	}
	yes, err = p.ConfirmCost(context.Background(), view, 100)
	if err != nil || !yes {
		t.Fatalf("explicit approval=%v err=%v", yes, err)
	}
	for _, value := range []string{"730", "8760", "100", "billing", "CUSTOM"} {
		if !strings.Contains(output.String(), value) {
			t.Fatalf("missing %q: %s", value, output.String())
		}
	}
}

func TestUseTerminalApplicationRejectsNonterminalStreams(t *testing.T) {
	if UseTerminalApplication(strings.NewReader(""), io.Discard, func(name string) string {
		if name == "TERM" {
			return "xterm-256color"
		}
		return ""
	}) {
		t.Fatal("pipe selected fullscreen")
	}
}

func TestHuhPrompterNonterminalSecretUsesSafeFallback(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("ACCESSIBLE", "")
	var output bytes.Buffer
	p := NewHuhPrompter(strings.NewReader("test-secret\n"), &output)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	token, err := p.VastAPIKey(ctx)
	if err != nil || token != "test-secret" {
		t.Fatalf("injected password failed: %v", err)
	}
	if strings.Contains(output.String(), "test-secret") || strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("unsafe password output: %q", output.String())
	}
}

func TestAccessibleSetupRedactsProviderErrors(t *testing.T) {
	for _, fromEnvironment := range []bool{false, true} {
		t.Run(fmt.Sprint(fromEnvironment), func(t *testing.T) {
			input := "1\n1\nsecret-token\n/key\n"
			if fromEnvironment {
				input = "1\n1\n/key\n"
			}
			var output bytes.Buffer
			err := Setup(context.Background(), strings.NewReader(input), &output, t.TempDir()+"/config.toml", "alice", SetupDependencies{
				Recipes: []recipe.Recipe{testRecipe()}, Getenv: func(key string) string {
					if fromEnvironment && key == "VAST_API_KEY" {
						return "secret-token"
					}
					return ""
				}, HomeDir: func() (string, error) { return "/fixture", nil },
				RunVast: func(_ context.Context, token, identity string, _ VastWorkload, _ setup.Operator) (setup.Result, error) {
					return setup.Result{}, fmt.Errorf("request rejected for %s", token)
				},
			})
			if err == nil || strings.Contains(err.Error(), "secret-token") || strings.Contains(output.String(), "secret-token") {
				t.Fatalf("unsafe provider diagnostic: %v", err)
			}
			if !strings.Contains(err.Error(), "[redacted]") {
				t.Fatalf("lost provider error: %v", err)
			}
		})
	}
}

func TestAccessibleCostPreservesCompleteDeploymentReview(t *testing.T) {
	var output bytes.Buffer
	p := NewAccessiblePrompter(strings.NewReader("yes\n"), &output)
	yes, err := p.ConfirmCost(context.Background(), rentalReviewFixture(), 120)
	if err != nil || !yes {
		t.Fatalf("confirmation failed: %v", err)
	}
	for _, want := range []string{"Offer 42", "Machine 88", "RadixArk/Qwen3.8-27B-NVFP4", "319f741cce68d7914884900c138a1fbb70a42f30", "120 GB allocated", "compute only", "excludes storage, egress, and tax", "billing starts immediately"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("missing %q: %s", want, output.String())
		}
	}
}

func TestAccessibleCustomSetupConsumesInjectedAnswersAcrossAllStages(t *testing.T) {
	path := t.TempDir() + "/config.toml"
	api := &applicationAPI{}
	var output bytes.Buffer
	input := strings.NewReader("1\n2\nowner/model\n48\n100\nyes\nprivate-api-key\n/fixture/key\n1\nyes\nyes\n")
	err := Setup(context.Background(), input, &output, path, "alice", SetupDependencies{
		Recipes: []recipe.Recipe{testRecipe()}, Getenv: func(string) string { return "" }, HomeDir: func() (string, error) { return "/fixture", nil },
		InspectModel: func(_ context.Context, repo string) (huggingface.Model, error) {
			if repo != "owner/model" {
				t.Fatalf("wrong inspected repo: %s", repo)
			}
			return huggingface.Model{Repository: repo, Revision: strings.Repeat("b", 40), Classification: catalog.Result{Status: catalog.Supported, Kind: "text-generation", Engine: "sglang"}}, nil
		},
		RunVast: func(ctx context.Context, token, identity string, workload VastWorkload, operator setup.Operator) (setup.Result, error) {
			if token != "private-api-key" || workload.AutoSelectOffer || workload.Recipe.Requirements.MinimumVRAMGB != 48 {
				t.Fatal("custom input was lost")
			}
			return setup.RunVast(ctx, token, workload.Recipe, setup.Options{ConfigPath: path, IdentityFile: identity, KnownHostsDir: t.TempDir(), AutoSelectOffer: workload.AutoSelectOffer, PollTimeout: time.Minute}, setup.Dependencies{
				NewAPI: func(string) setup.VastAPI { return api }, Operator: operator, HostKeyScanner: applicationScanner{}, TrustStore: applicationTrust{}, ServerLauncher: applicationServer{}, Clock: setup.RealClock{}, SaveConfig: config.Save, ValidateIdentity: func(string) error { return nil },
			})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if api.creates.Load() != 1 {
		t.Fatalf("creates=%d", api.creates.Load())
	}
	if _, err := config.Load(path); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "private-api-key") || strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("unsafe fallback output: %s", output.String())
	}
	for _, want := range []string{"owner/model", "billing"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("missing decision detail %q", want)
		}
	}
}
