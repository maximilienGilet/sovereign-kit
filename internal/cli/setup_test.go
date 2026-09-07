package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maximilienGilet/sovereign-kit/internal/catalog"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/huggingface"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

type fakeSetupPrompter struct {
	provider                string
	manual                  ManualRoute
	apiKey                  string
	apiKeyErr               error
	apiKeyCalls             int
	identity                string
	identityInput           string
	selectOfferErr          error
	workload                string
	hfQuery                 string
	hfModel                 string
	hardware                CustomHardware
	events                  *[]string
	customConfirmCalls      int
	customConfirmedModel    huggingface.Model
	customConfirmedHardware CustomHardware
	declineCustom           bool
}

func (p *fakeSetupPrompter) SelectProvider(context.Context) (string, error) {
	if p.events != nil {
		*p.events = append(*p.events, "select-provider")
	}
	return p.provider, nil
}

func (p *fakeSetupPrompter) VastAPIKey(context.Context) (string, error) {
	p.apiKeyCalls++
	if p.events != nil {
		*p.events = append(*p.events, "api-key")
	}
	return p.apiKey, p.apiKeyErr
}

func (p *fakeSetupPrompter) ManualRoute(context.Context, string) (ManualRoute, error) {
	return p.manual, nil
}

func (p *fakeSetupPrompter) VastIdentity(_ context.Context, defaultValue string) (string, error) {
	p.identityInput = defaultValue
	if p.events != nil {
		*p.events = append(*p.events, "identity")
	}
	if p.identity == "" {
		return defaultValue, nil
	}
	return p.identity, nil
}

func (p *fakeSetupPrompter) SelectOffer(context.Context, []setup.OfferView) (vast.Offer, error) {
	return vast.Offer{}, p.selectOfferErr
}

func (p *fakeSetupPrompter) SelectWorkload(_ context.Context, recipes []recipe.Recipe) (string, error) {
	if p.events != nil {
		*p.events = append(*p.events, "select-workload")
	}
	if p.workload != "" {
		return p.workload, nil
	}
	return recipes[0].ID, nil
}

func (p *fakeSetupPrompter) HuggingFaceQuery(context.Context) (string, error) {
	return p.hfQuery, nil
}

func (p *fakeSetupPrompter) SelectHuggingFaceModel(context.Context, []huggingface.SearchResult) (string, error) {
	return p.hfModel, nil
}

func (p *fakeSetupPrompter) CustomHardware(context.Context) (CustomHardware, error) {
	return p.hardware, nil
}

func (p *fakeSetupPrompter) ConfirmCustomWorkload(_ context.Context, model huggingface.Model, hardware CustomHardware) (bool, error) {
	p.customConfirmCalls++
	p.customConfirmedModel = model
	p.customConfirmedHardware = hardware
	if p.events != nil {
		*p.events = append(*p.events, "confirm-custom")
	}
	return !p.declineCustom, nil
}

func (p *fakeSetupPrompter) ConfirmCost(context.Context, setup.OfferView, int) (bool, error) {
	return true, nil
}

func (p *fakeSetupPrompter) ConfirmHostKeys(context.Context, []string) (bool, error) {
	return true, nil
}

type vastOffer struct{}

func TestSetupKeepsManualSecureRoute(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	identity := filepath.Join(dir, "identity")
	knownHosts := filepath.Join(dir, "known_hosts")
	if err := os.WriteFile(identity, []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(knownHosts, []byte("host ssh-ed25519 AAAA"), 0o600); err != nil {
		t.Fatal(err)
	}
	prompter := &fakeSetupPrompter{
		provider: "manual",
		manual:   ManualRoute{Host: "gpu.example.test", Port: 2222, User: "sovkit", IdentityFile: identity, KnownHostsFile: knownHosts},
	}
	getenvCalled := false
	deps := SetupDependencies{
		Prompter: prompter,
		Getenv: func(string) string {
			getenvCalled = true
			return "should-not-be-read"
		},
		RunVast: func(context.Context, string, string, VastWorkload, setup.Operator) (setup.Result, error) {
			return setup.Result{}, errors.New("Vast runner must not be called")
		},
	}

	if err := Setup(context.Background(), bytes.NewBuffer(nil), &bytes.Buffer{}, configPath, "alice", deps); err != nil {
		t.Fatal(err)
	}
	if getenvCalled {
		t.Fatal("manual setup read VAST_API_KEY")
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider.Kind != "manual" || cfg.Route.LocalHost != "127.0.0.1" || cfg.Route.LocalPort != 30000 || cfg.Route.RemoteHost != "127.0.0.1" || cfg.Route.RemotePort != 30000 {
		t.Fatalf("unexpected route config: %#v", cfg)
	}
	if cfg.SSH.Host != prompter.manual.Host || cfg.SSH.Port != prompter.manual.Port || cfg.SSH.User != prompter.manual.User || cfg.SSH.IdentityFile != identity || cfg.SSH.KnownHostsFile != knownHosts {
		t.Fatalf("unexpected SSH config: %#v", cfg.SSH)
	}
}

func testRecipe() recipe.Recipe {
	return recipe.Recipe{
		Version: 1,
		ID:      "qwen-studio",
		Name:    "Qwen Studio",
		Kind:    "text-generation",
		Runtime: recipe.Runtime{Engine: "sglang", Image: "runtime@example@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		Model:   recipe.Model{Repository: "Qwen/model", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		Serve:   recipe.Serve{ContextWindow: 32768, MaxOutputTokens: 4096, MaxRunningRequests: 1},
		Requirements: recipe.Requirements{
			MinimumVRAMGB: 96,
			MinimumDiskGB: 120,
		},
	}
}

func TestResolveVastWorkloadSelectsRecipeWithoutModelOrHardwarePrompts(t *testing.T) {
	selectedRecipe := testRecipe()
	prompter := &fakeSetupPrompter{workload: selectedRecipe.ID}
	searchCalls, inspectCalls := 0, 0
	workload, err := resolveVastWorkload(
		context.Background(),
		prompter,
		[]recipe.Recipe{selectedRecipe},
		func(context.Context, string, int) ([]huggingface.SearchResult, error) {
			searchCalls++
			return nil, nil
		},
		func(context.Context, string) (huggingface.Model, error) {
			inspectCalls++
			return huggingface.Model{}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if workload.Recipe.ID != selectedRecipe.ID || workload.AutoSelectOffer || searchCalls != 0 || inspectCalls != 0 {
		t.Fatalf("workload=%#v search=%d inspect=%d", workload, searchCalls, inspectCalls)
	}
}

func TestResolveVastWorkloadInspectsDirectModelAndAppliesHardware(t *testing.T) {
	const revision = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	prompter := &fakeSetupPrompter{
		workload: customHuggingFaceWorkload,
		hfQuery:  "Qwen/custom-model",
		hardware: CustomHardware{MinimumVRAMGB: 48, MinimumDiskGB: 100},
	}
	searchCalls := 0
	workload, err := resolveVastWorkload(
		context.Background(),
		prompter,
		[]recipe.Recipe{testRecipe()},
		func(context.Context, string, int) ([]huggingface.SearchResult, error) {
			searchCalls++
			return nil, nil
		},
		func(_ context.Context, repository string) (huggingface.Model, error) {
			if repository != "Qwen/custom-model" {
				t.Fatalf("repository=%q", repository)
			}
			return huggingface.Model{
				Repository: repository,
				Revision:   revision,
				Classification: catalog.Result{
					Kind:   "text-generation",
					Engine: "sglang",
					Status: catalog.Supported,
				},
			}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if searchCalls != 0 || workload.AutoSelectOffer || workload.Recipe.Model.Repository != "Qwen/custom-model" || workload.Recipe.Model.Revision != revision {
		t.Fatalf("workload=%#v search=%d", workload, searchCalls)
	}
	if workload.Recipe.Requirements.MinimumVRAMGB != 48 || workload.Recipe.Requirements.MinimumDiskGB != 100 {
		t.Fatalf("requirements=%#v", workload.Recipe.Requirements)
	}
	if prompter.customConfirmCalls != 1 || prompter.customConfirmedModel.Repository != "Qwen/custom-model" || prompter.customConfirmedModel.Revision != revision || prompter.customConfirmedHardware != prompter.hardware {
		t.Fatalf("custom confirmation calls=%d model=%#v hardware=%#v", prompter.customConfirmCalls, prompter.customConfirmedModel, prompter.customConfirmedHardware)
	}
}

func TestSetupVastDecliningCustomWorkloadAvoidsProviderCalls(t *testing.T) {
	getenvCalls, runnerCalls := 0, 0
	prompter := &fakeSetupPrompter{
		provider:      "vast",
		workload:      customHuggingFaceWorkload,
		hfQuery:       "Qwen/custom-model",
		hardware:      CustomHardware{MinimumVRAMGB: 48, MinimumDiskGB: 100},
		declineCustom: true,
	}
	deps := SetupDependencies{
		Prompter: prompter,
		Recipes:  []recipe.Recipe{testRecipe()},
		Getenv: func(string) string {
			getenvCalls++
			return "secret"
		},
		HomeDir: func() (string, error) { return "/home/alice", nil },
		InspectModel: func(_ context.Context, repository string) (huggingface.Model, error) {
			return huggingface.Model{
				Repository: repository,
				Revision:   "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				Classification: catalog.Result{
					Kind:   "text-generation",
					Engine: "sglang",
					Status: catalog.Supported,
				},
			}, nil
		},
		RunVast: func(context.Context, string, string, VastWorkload, setup.Operator) (setup.Result, error) {
			runnerCalls++
			return setup.Result{}, nil
		},
	}
	err := Setup(context.Background(), bytes.NewBuffer(nil), &bytes.Buffer{}, "config.toml", "alice", deps)
	if err == nil || !strings.Contains(err.Error(), "cancel") {
		t.Fatalf("error=%v", err)
	}
	if prompter.customConfirmCalls != 1 || getenvCalls != 0 || prompter.apiKeyCalls != 0 || runnerCalls != 0 {
		t.Fatalf("confirm=%d getenv=%d api-key=%d runner=%d", prompter.customConfirmCalls, getenvCalls, prompter.apiKeyCalls, runnerCalls)
	}
}

func TestResolveVastWorkloadSearchesAndUsesSelectedModel(t *testing.T) {
	const selected = "Qwen/Qwen3-Coder"
	prompter := &fakeSetupPrompter{
		workload: customHuggingFaceWorkload,
		hfQuery:  "qwen coder",
		hfModel:  selected,
		hardware: CustomHardware{MinimumVRAMGB: 80, MinimumDiskGB: 100},
	}
	inspected := ""
	workload, err := resolveVastWorkload(
		context.Background(),
		prompter,
		[]recipe.Recipe{testRecipe()},
		func(_ context.Context, query string, limit int) ([]huggingface.SearchResult, error) {
			if query != "qwen coder" || limit != 10 {
				t.Fatalf("query=%q limit=%d", query, limit)
			}
			return []huggingface.SearchResult{{Repository: selected}}, nil
		},
		func(_ context.Context, repository string) (huggingface.Model, error) {
			inspected = repository
			return huggingface.Model{
				Repository: repository,
				Revision:   "cccccccccccccccccccccccccccccccccccccccc",
				Classification: catalog.Result{
					Kind:   "text-generation",
					Engine: "sglang",
					Status: catalog.Supported,
				},
			}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if inspected != selected || workload.Recipe.Model.Repository != selected || workload.AutoSelectOffer {
		t.Fatalf("inspected=%q workload=%#v", inspected, workload)
	}
}

func TestSetupVastResolvesWorkloadBeforeCredentials(t *testing.T) {
	events := []string{}
	selectedRecipe := testRecipe()
	prompter := &fakeSetupPrompter{
		provider: "vast",
		workload: selectedRecipe.ID,
		apiKey:   "secret",
		identity: "/home/alice/.ssh/key",
		events:   &events,
	}
	deps := SetupDependencies{
		Prompter: prompter,
		Recipes:  []recipe.Recipe{selectedRecipe},
		Getenv:   func(string) string { return "" },
		HomeDir:  func() (string, error) { return "/home/alice", nil },
		RunVast: func(_ context.Context, _ string, _ string, workload VastWorkload, _ setup.Operator) (setup.Result, error) {
			events = append(events, "run-vast")
			if workload.Recipe.ID != selectedRecipe.ID || workload.AutoSelectOffer {
				t.Fatalf("workload=%#v", workload)
			}
			return setup.Result{InstanceID: 1, ConfigPath: "config.toml"}, nil
		},
	}

	if err := Setup(context.Background(), bytes.NewBuffer(nil), &bytes.Buffer{}, "config.toml", "alice", deps); err != nil {
		t.Fatal(err)
	}
	want := []string{"select-provider", "select-workload", "api-key", "identity", "run-vast"}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("events=%v want=%v", events, want)
	}
}

func TestSetupVastPromptsForMissingAPIKey(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	prompter := &fakeSetupPrompter{provider: "vast", apiKey: " prompted-secret ", identity: "/home/alice/.ssh/id_ed25519"}
	var gotToken string
	deps := SetupDependencies{
		Prompter: prompter,
		Recipes:  []recipe.Recipe{testRecipe()},
		Getenv: func(key string) string {
			if key != "VAST_API_KEY" {
				t.Fatalf("unexpected environment lookup %q", key)
			}
			return ""
		},
		HomeDir: func() (string, error) { return "/home/alice", nil },
		RunVast: func(_ context.Context, token, _ string, _ VastWorkload, _ setup.Operator) (setup.Result, error) {
			gotToken = token
			return setup.Result{InstanceID: 41, ConfigPath: configPath}, nil
		},
	}

	if err := Setup(context.Background(), bytes.NewBuffer(nil), &bytes.Buffer{}, configPath, "alice", deps); err != nil {
		t.Fatal(err)
	}
	if prompter.apiKeyCalls != 1 || gotToken != "prompted-secret" {
		t.Fatalf("API key prompts=%d token=%q", prompter.apiKeyCalls, gotToken)
	}
}

func TestSetupVastExplainsHowToProvideAPIKeyWhenPromptFails(t *testing.T) {
	prompter := &fakeSetupPrompter{provider: "vast", apiKeyErr: errors.New("input closed")}
	runnerCalled := false
	deps := SetupDependencies{
		Prompter: prompter,
		Recipes:  []recipe.Recipe{testRecipe()},
		Getenv:   func(string) string { return "" },
		RunVast: func(context.Context, string, string, VastWorkload, setup.Operator) (setup.Result, error) {
			runnerCalled = true
			return setup.Result{}, nil
		},
	}
	err := Setup(context.Background(), bytes.NewBuffer(nil), &bytes.Buffer{}, filepath.Join(t.TempDir(), "config.toml"), "alice", deps)
	if err == nil || !strings.Contains(err.Error(), "https://console.vast.ai/keys") || !strings.Contains(err.Error(), "VAST_API_KEY") {
		t.Fatalf("missing provider guidance: %v", err)
	}
	if runnerCalled {
		t.Fatal("Vast runner was called without an API key")
	}
}

func TestSetupVastDefaultsToDedicatedIdentityPath(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	prompter := &fakeSetupPrompter{provider: "vast"}
	var gotToken, gotIdentity string
	deps := SetupDependencies{
		Prompter: prompter,
		Recipes:  []recipe.Recipe{testRecipe()},
		Getenv:   func(string) string { return "exact-secret-token" },
		HomeDir:  func() (string, error) { return "/home/alice", nil },
		RunVast: func(_ context.Context, token, identity string, _ VastWorkload, _ setup.Operator) (setup.Result, error) {
			gotToken, gotIdentity = token, identity
			return setup.Result{InstanceID: 41, ConfigPath: configPath}, nil
		},
	}
	if err := Setup(context.Background(), bytes.NewBuffer(nil), &bytes.Buffer{}, configPath, "alice", deps); err != nil {
		t.Fatal(err)
	}
	wantIdentity := filepath.Join("/home/alice", ".ssh", "sovkit_vast_ed25519")
	if prompter.identityInput != wantIdentity {
		t.Fatalf("unexpected identity default %q", prompter.identityInput)
	}
	if gotToken != "exact-secret-token" || gotIdentity != wantIdentity {
		t.Fatalf("unexpected runner args token=%q identity=%q", gotToken, gotIdentity)
	}
	if prompter.apiKeyCalls != 0 {
		t.Fatalf("prompted for API key despite VAST_API_KEY, calls=%d", prompter.apiKeyCalls)
	}
}
func TestSetupVastPrintsResultAndStartNextStep(t *testing.T) {
	output := &bytes.Buffer{}
	configPath := filepath.Join(t.TempDir(), "custom-config.toml")
	deps := SetupDependencies{
		Prompter: &fakeSetupPrompter{provider: "vast", identity: "/home/alice/.ssh/id_ed25519"},
		Recipes:  []recipe.Recipe{testRecipe()},
		Getenv:   func(string) string { return "secret-token" },
		HomeDir:  func() (string, error) { return "/home/alice", nil },
		RunVast: func(context.Context, string, string, VastWorkload, setup.Operator) (setup.Result, error) {
			return setup.Result{InstanceID: 987, ConfigPath: configPath}, nil
		},
	}
	if err := Setup(context.Background(), bytes.NewBuffer(nil), output, configPath, "alice", deps); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	want := "Sovereign Kit setup\nVast instance 987 created.\nConfiguration saved: " + configPath + "\nWarning: billing may still be active for this instance.\nNext: sovkit start\n"
	if text != want {
		t.Fatalf("output = %q, want %q", text, want)
	}
	if strings.Contains(text, "secret-token") {
		t.Fatal("output leaked API token")
	}
}

func TestSetupPrintsTitleBeforeProviderPrompt(t *testing.T) {
	events := []string{}
	prompter := &fakeSetupPrompter{provider: "vast", events: &events}
	output := eventWriter{events: &events}
	deps := SetupDependencies{
		Prompter: prompter,
		Getenv:   func(string) string { return "" },
	}
	if err := Setup(context.Background(), bytes.NewBuffer(nil), output, filepath.Join(t.TempDir(), "config.toml"), "alice", deps); err == nil {
		t.Fatal("expected missing API key error")
	}
	want := []string{"output", "select-provider"}
	if len(events) < len(want) || events[0] != want[0] || events[1] != want[1] {
		t.Fatalf("events = %#v, want prefix %#v", events, want)
	}
}

type eventWriter struct {
	events *[]string
}

func (w eventWriter) Write(data []byte) (int, error) {
	*w.events = append(*w.events, "output")
	return len(data), nil
}

func TestSetupManualValidatesBothCredentialFiles(t *testing.T) {
	for _, field := range []string{"identity", "known-hosts"} {
		t.Run(field, func(t *testing.T) {
			dir := t.TempDir()
			identity := filepath.Join(dir, "identity")
			knownHosts := filepath.Join(dir, "known_hosts")
			if field == "identity" {
				if err := os.WriteFile(knownHosts, []byte("host ssh-ed25519 AAAA"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(identity, []byte("key"), 0o600); err != nil {
				t.Fatal(err)
			}
			deps := SetupDependencies{
				Prompter: &fakeSetupPrompter{provider: "manual", manual: ManualRoute{Host: "gpu", Port: 22, User: "alice", IdentityFile: identity, KnownHostsFile: knownHosts}},
			}
			if err := Setup(context.Background(), bytes.NewBuffer(nil), &bytes.Buffer{}, filepath.Join(dir, "config.toml"), "alice", deps); err == nil {
				t.Fatal("expected missing credential file error")
			}
		})
	}
}
