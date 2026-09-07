package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/maximilienGilet/sovereign-kit/internal/catalog"
	"github.com/maximilienGilet/sovereign-kit/internal/catalogui"
	"github.com/maximilienGilet/sovereign-kit/internal/huggingface"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

func TestProviderOptionsUseStableLabels(t *testing.T) {
	options := providerOptions()
	if len(options) != 2 {
		t.Fatalf("expected two providers, got %d", len(options))
	}
	if options[0].Key != "Vast" || options[0].Value != "vast" {
		t.Fatalf("unexpected Vast option: %#v", options[0])
	}
	if options[1].Key != "Manual SSH" || options[1].Value != "manual" {
		t.Fatalf("unexpected manual option: %#v", options[1])
	}
}

func TestProviderPromptExplainsBothSetupPaths(t *testing.T) {
	provider := ""
	view := providerSelectField(&provider).View()
	for _, expected := range []string{"Vast creates a paid GPU instance", "Manual SSH configures an existing GPU host"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("provider prompt missing %q:\n%s", expected, view)
		}
	}
}

func TestWorkloadOptionsShowRecipeStatusAndExactGPU(t *testing.T) {
	options := workloadOptions([]recipe.Recipe{{
		ID:      "qwen-solo-rtx5090",
		Name:    "Qwen Solo",
		Profile: recipe.Profile{Status: "experimental"},
		Serve:   recipe.Serve{ContextWindow: 32768},
		Requirements: recipe.Requirements{
			GPUModel: "RTX 5090",
			GPUCount: 1,
		},
	}})
	if len(options) != 2 || options[0].Value != "qwen-solo-rtx5090" {
		t.Fatalf("unexpected recipe option: %#v", options)
	}
	for _, want := range []string{"Recipe · Qwen Solo", "EXPERIMENTAL", "1× RTX 5090", "32,768 context"} {
		if !strings.Contains(options[0].Key, want) {
			t.Fatalf("recipe option missing %q: %s", want, options[0].Key)
		}
	}
	if options[1].Value != customHuggingFaceWorkload {
		t.Fatalf("unexpected custom option: %#v", options[1])
	}
	for _, want := range []string{"Hugging Face model", "CUSTOM", "unknown/unmeasured GPU", "unknown/unmeasured context"} {
		if !strings.Contains(options[1].Key, want) {
			t.Fatalf("custom option missing %q: %s", want, options[1].Key)
		}
	}
}

func TestWorkloadEntriesKeepStableRecipeAndCustomValues(t *testing.T) {
	available := []recipe.Recipe{{
		ID: "qwen-solo-rtx5090", Name: "Qwen Solo",
		Profile: recipe.Profile{Status: "experimental", UseWhen: []string{"One private session"}},
		Model:   recipe.Model{NativeContextWindow: 262144},
		Serve:   recipe.Serve{ContextWindow: 32768},
	}}
	entries := workloadEntries(available)
	if len(entries) != 2 || entries[0].Value != available[0].ID ||
		entries[1].Value != customHuggingFaceWorkload || !entries[1].Custom {
		t.Fatalf("unexpected entries: %#v", entries)
	}
}

func TestRichWorkloadPickerReturnsSelectedStableValue(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("ACCESSIBLE", "")
	available := []recipe.Recipe{{ID: "studio", Name: "Studio"}, {ID: "solo", Name: "Solo"}}
	prompter := NewHuhPrompter(bytes.NewReader(nil), io.Discard)
	prompter.runRecipePicker = func(_ context.Context, model catalogui.Model, _ io.Reader, _ io.Writer) (catalogui.Model, error) {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
		updated, _ = updated.(catalogui.Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
		return updated.(catalogui.Model), nil
	}
	selected, err := prompter.SelectWorkload(context.Background(), available)
	if err != nil || selected != "solo" {
		t.Fatalf("selected=%q err=%v", selected, err)
	}
}

func TestRichWorkloadPickerMapsCancellationAndRuntimeErrors(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("ACCESSIBLE", "")
	available := []recipe.Recipe{{ID: "studio", Name: "Studio"}}
	for _, test := range []struct {
		name   string
		runner recipePickerRunner
		want   string
	}{
		{
			name: "cancel",
			runner: func(_ context.Context, model catalogui.Model, _ io.Reader, _ io.Writer) (catalogui.Model, error) {
				updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
				return updated.(catalogui.Model), nil
			},
			want: "setup cancelled",
		},
		{
			name: "runtime error",
			runner: func(_ context.Context, model catalogui.Model, _ io.Reader, _ io.Writer) (catalogui.Model, error) {
				return model, errors.New("terminal failed")
			},
			want: "choose workload: terminal failed",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			prompter := NewHuhPrompter(bytes.NewReader(nil), io.Discard)
			prompter.runRecipePicker = test.runner
			_, err := prompter.SelectWorkload(context.Background(), available)
			if err == nil || err.Error() != test.want {
				t.Fatalf("error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestWorkloadUsesHuhFallbackForAccessibleEnvironments(t *testing.T) {
	for _, test := range []struct {
		name   string
		values map[string]string
		want   bool
	}{
		{name: "rich", values: map[string]string{"TERM": "xterm-256color"}, want: false},
		{name: "dumb terminal", values: map[string]string{"TERM": "dumb"}, want: true},
		{name: "accessible", values: map[string]string{"TERM": "xterm-256color", "ACCESSIBLE": "1"}, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			getenv := func(name string) string { return test.values[name] }
			if got := useAccessibleWorkload(getenv); got != test.want {
				t.Fatalf("accessible=%t, want %t", got, test.want)
			}
		})
	}
}

func TestHuggingFacePromptSupportsSearchAndDirectRepository(t *testing.T) {
	query := ""
	view := huggingFaceQueryField(&query).View()
	for _, expected := range []string{"Search", "owner/model"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("model prompt missing %q:\n%s", expected, view)
		}
	}
}

func TestHuggingFaceModelOptionsShowPopularity(t *testing.T) {
	options := huggingFaceModelOptions([]huggingface.SearchResult{{
		Repository: "Qwen/Qwen3-Coder-30B-A3B-Instruct",
		Downloads:  1200,
		Likes:      75,
	}})
	if len(options) != 1 || options[0].Value != "Qwen/Qwen3-Coder-30B-A3B-Instruct" {
		t.Fatalf("unexpected model options: %#v", options)
	}
	for _, expected := range []string{"1,200 downloads", "75 likes"} {
		if !strings.Contains(options[0].Key, expected) {
			t.Fatalf("model option missing %q: %s", expected, options[0].Key)
		}
	}
}

func TestCustomHardwareDefaultsDiskTo100GB(t *testing.T) {
	vram, disk := "", ""
	fields := customHardwareFields(&vram, &disk)
	if disk != "100" || len(fields) != 2 {
		t.Fatalf("disk=%q fields=%d", disk, len(fields))
	}
}

func TestCustomWorkloadConfirmationShowsPinnedClassificationAndHardware(t *testing.T) {
	title := customWorkloadConfirmationTitle(huggingface.Model{
		Repository: "Qwen/custom-model",
		Revision:   "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Classification: catalog.Result{
			Kind:   "text-generation",
			Engine: "sglang",
			Status: catalog.Supported,
		},
	}, CustomHardware{MinimumVRAMGB: 48, MinimumDiskGB: 100})
	for _, want := range []string{"Qwen/custom-model", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "text-generation", "sglang", "supported", "48 GB VRAM", "100 GB disk"} {
		if !strings.Contains(title, want) {
			t.Fatalf("custom confirmation missing %q: %s", want, title)
		}
	}
}

func TestVastAPIKeyFieldGuidesAndMasks(t *testing.T) {
	token := "top-secret-token"
	field := vastAPIKeyField(&token)
	view := field.View()
	if !strings.Contains(view, "https://console.vast.ai/keys") {
		t.Fatalf("API key prompt lacks provider guidance:\n%s", view)
	}
	if strings.Contains(view, token) {
		t.Fatalf("API key prompt exposed token:\n%s", view)
	}
}

func TestVastIdentityPromptExplainsAutomaticSetup(t *testing.T) {
	identity := "/home/alice/.ssh/sovkit_vast_ed25519"
	view := strings.ToLower(vastIdentityField(&identity).View())
	for _, expected := range []string{"generates", "public key", "vast"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("identity prompt missing %q:\n%s", expected, view)
		}
	}
}

func TestIdentitySetupConfirmationNamesMutations(t *testing.T) {
	path := "/home/alice/.ssh/sovkit_vast_ed25519"
	generate := strings.ToLower(identitySetupTitle(path, true))
	for _, expected := range []string{"generate", path, "register", "vast account"} {
		if !strings.Contains(generate, expected) {
			t.Fatalf("generation confirmation missing %q: %s", expected, generate)
		}
	}
	register := strings.ToLower(identitySetupTitle(path, false))
	if strings.Contains(register, "generate") || !strings.Contains(register, "register") || !strings.Contains(register, "vast account") {
		t.Fatalf("registration confirmation is unclear: %s", register)
	}
}

func TestSetupKeyMapsAcceptTerminalEnterAndProviderSpace(t *testing.T) {
	keyMap := setupKeyMap()
	for _, message := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyCtrlJ}} {
		if !key.Matches(message, keyMap.Select.Submit) {
			t.Fatalf("select submit does not accept %q", message.String())
		}
		if !key.Matches(message, keyMap.Input.Submit) {
			t.Fatalf("input submit does not accept %q", message.String())
		}
	}
	if key.Matches(tea.KeyMsg{Type: tea.KeyCtrlJ}, keyMap.Select.Down) {
		t.Fatal("terminal enter is still bound to select down")
	}
	if key.Matches(tea.KeyMsg{Type: tea.KeySpace}, keyMap.Select.Submit) {
		t.Fatal("filterable selects submit on space")
	}
	if !key.Matches(tea.KeyMsg{Type: tea.KeySpace}, providerKeyMap().Select.Submit) {
		t.Fatal("provider select does not accept space")
	}
	if !key.Matches(tea.KeyMsg{Type: tea.KeySpace}, keyMap.Confirm.Toggle) {
		t.Fatal("confirm toggle does not accept space")
	}
}

func TestOfferSelectFilterPreservesSpaces(t *testing.T) {
	selected := 0
	field := huh.NewSelect[int]().
		Title("Choose a Vast GPU offer").
		Options(
			huh.NewOption("RTX 4090 · 48 GB", 1),
			huh.NewOption("RTX 6000 · 96 GB", 2),
		).
		Value(&selected)
	form := huh.NewForm(huh.NewGroup(field)).WithKeyMap(setupKeyMap())
	form.Update(form.Init())
	form.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	form.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("RTX")})
	form.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	form.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("4090")})
	if form.State == huh.StateCompleted {
		t.Fatal("space submitted the offer select while filtering")
	}
	if view := field.View(); !strings.Contains(view, "RTX 4090") {
		t.Fatalf("space was not preserved in the offer filter:\n%s", view)
	}
}

func TestOfferLabelsShowCostCapacityAndReliability(t *testing.T) {
	view := setup.OfferView{
		Offer:      vast.Offer{ID: 7, GPUName: "RTX 4090", GPUVRAMGB: 47.5, HourlyUSD: 1.234, Location: "US", Reliability: 0.987},
		MonthlyUSD: 900.82,
		AnnualUSD:  10810.44,
	}
	label := offerLabel(view)
	for _, want := range []string{"RTX 4090", "47.5 GB VRAM", "$1.23/h", "Monthly $900.82", "Annual $10810.44", "US", "98.7%"} {
		if !strings.Contains(label, want) {
			t.Fatalf("offer label missing %q: %s", want, label)
		}
	}
}

func TestOfferLabelsMarkUnknownReliability(t *testing.T) {
	label := offerLabel(setup.OfferView{Offer: vast.Offer{GPUVRAMGB: 0, Reliability: 0}})
	if !strings.Contains(label, "unknown/unmeasured") {
		t.Fatalf("expected unknown reliability marker: %s", label)
	}
}

func TestOfferOptionsUseOfferIDs(t *testing.T) {
	options := offerOptions([]setup.OfferView{{Offer: vast.Offer{ID: 9, GPUName: "A"}}, {Offer: vast.Offer{ID: 3, GPUName: "B"}}})
	if len(options) != 2 || options[0].Value != 9 || options[1].Value != 3 {
		t.Fatalf("unexpected offer options: %#v", options)
	}
}

func TestOfferLabelsNameComputePeriods(t *testing.T) {
	label := offerLabel(setup.OfferView{Offer: vast.Offer{HourlyUSD: 1}, MonthlyUSD: 730, AnnualUSD: 8760})
	for _, want := range []string{"730h monthly compute", "8,760h annual compute", "Monthly $730.00", "Annual $8760.00"} {
		if !strings.Contains(label, want) {
			t.Fatalf("offer label missing %q: %s", want, label)
		}
	}
}

func TestMapFormErrorMapsAbortAndPassesThrough(t *testing.T) {
	prompter := &HuhPrompter{}
	if err := prompter.mapFormError(huh.ErrUserAborted); err == nil || err.Error() != "setup cancelled" {
		t.Fatalf("abort error = %v, want setup cancelled", err)
	}
	other := errors.New("other")
	if err := prompter.mapFormError(other); err != other {
		t.Fatalf("other error = %v, want original error", err)
	}
}

func TestSetupRejectsUnknownProvider(t *testing.T) {
	err := Setup(context.Background(), strings.NewReader(""), &strings.Builder{}, "/tmp/config.toml", "alice", SetupDependencies{
		Prompter: &fakeSetupPrompter{provider: "unknown"},
	})
	if err == nil || !strings.Contains(err.Error(), `unknown setup provider "unknown"`) {
		t.Fatalf("error = %v, want unknown provider rejection", err)
	}
}

func TestSetupManualRejectsOutOfRangePorts(t *testing.T) {
	for _, port := range []int{0, 65536} {
		t.Run(strconv.Itoa(port), func(t *testing.T) {
			dir := t.TempDir()
			identity := filepath.Join(dir, "identity")
			knownHosts := filepath.Join(dir, "known_hosts")
			if err := os.WriteFile(identity, []byte("key"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(knownHosts, []byte("host ssh-ed25519 AAAA"), 0o600); err != nil {
				t.Fatal(err)
			}
			err := setupManual(context.Background(), &strings.Builder{}, filepath.Join(dir, "config.toml"), "alice", &fakeSetupPrompter{
				manual: ManualRoute{Host: "gpu", Port: port, User: "alice", IdentityFile: identity, KnownHostsFile: knownHosts},
			})
			if err == nil || !strings.Contains(err.Error(), "between 1 and 65535") {
				t.Fatalf("error = %v, want port bounds rejection", err)
			}
		})
	}
}

func TestHuhPrompterImplementsSetupInterfaces(t *testing.T) {
	var _ SetupPrompter = (*HuhPrompter)(nil)
	var _ setup.Operator = (*HuhPrompter)(nil)
	var _ = huh.ErrUserAborted
}

func TestManualRouteFieldsHaveOnePrefilledSSHUser(t *testing.T) {
	route := ManualRoute{}
	portText := "22"
	fields := manualRouteFields(&route, &portText, "alice")
	if len(fields) != 5 {
		t.Fatalf("expected five manual route inputs, got %d", len(fields))
	}
	if route.User != "alice" {
		t.Fatalf("expected default SSH user, got %q", route.User)
	}
	if got := fields[2].GetValue(); got != "alice" {
		t.Fatalf("expected prefilled SSH user field, got %#v", got)
	}
	if view := fields[2].View(); !strings.Contains(view, "alice") {
		t.Fatalf("SSH user view = %q, want alice", view)
	}
	count := 0
	for _, field := range fields {
		count += strings.Count(field.View(), "SSH user")
	}
	if count != 1 {
		t.Fatalf("expected one SSH user prompt, got %d", count)
	}
}
