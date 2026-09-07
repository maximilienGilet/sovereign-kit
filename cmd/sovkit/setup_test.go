package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/maximilienGilet/sovereign-kit/internal/cli"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/huggingface"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

type manualSetupPrompter struct {
	route cli.ManualRoute
}

func (p manualSetupPrompter) SelectProvider(context.Context) (string, error) {
	return "manual", nil
}

func (manualSetupPrompter) VastAPIKey(context.Context) (string, error) {
	return "", nil
}

func (p manualSetupPrompter) ManualRoute(context.Context, string) (cli.ManualRoute, error) {
	return p.route, nil
}

func (manualSetupPrompter) VastIdentity(context.Context, string) (string, error) {
	return "", nil
}

type vastSetupPrompter struct {
	identity string
	workload string
}

func (p vastSetupPrompter) SelectProvider(context.Context) (string, error) {
	return "vast", nil
}

func (vastSetupPrompter) VastAPIKey(context.Context) (string, error) {
	return "", nil
}

func (vastSetupPrompter) ManualRoute(context.Context, string) (cli.ManualRoute, error) {
	return cli.ManualRoute{}, nil
}

func (p vastSetupPrompter) VastIdentity(context.Context, string) (string, error) {
	return p.identity, nil
}

func (p vastSetupPrompter) SelectWorkload(_ context.Context, available []recipe.Recipe) (string, error) {
	if p.workload != "" {
		return p.workload, nil
	}
	return available[0].ID, nil
}

func (vastSetupPrompter) HuggingFaceQuery(context.Context) (string, error) {
	return "", nil
}

func (vastSetupPrompter) SelectHuggingFaceModel(context.Context, []huggingface.SearchResult) (string, error) {
	return "", nil
}

func (vastSetupPrompter) CustomHardware(context.Context) (cli.CustomHardware, error) {
	return cli.CustomHardware{}, nil
}

func (vastSetupPrompter) ConfirmCustomWorkload(context.Context, huggingface.Model, cli.CustomHardware) (bool, error) {
	return true, nil
}

func (vastSetupPrompter) ConfirmIdentitySetup(context.Context, string, bool) (bool, error) {
	return true, nil
}

func (vastSetupPrompter) SelectOffer(context.Context, []setup.OfferView) (vast.Offer, error) {
	return vast.Offer{ID: 1}, nil
}

func (vastSetupPrompter) ConfirmCost(context.Context, setup.OfferView, int) (bool, error) {
	return true, nil
}

func (vastSetupPrompter) ConfirmHostKeys(context.Context, []string) (bool, error) {
	return true, nil
}

func TestProductionSetupPassesPromptedIdentityToVastOptions(t *testing.T) {
	identity := filepath.Join(t.TempDir(), "identity")
	if err := os.WriteFile(identity, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	var gotOptions setup.Options
	events := []string{}
	var validated string
	app := productionApplicationWith(productionDependencies{
		Prompter: vastSetupPrompter{identity: identity},
		Getenv: func(string) string {
			return "test-token"
		},
		HomeDir: func() (string, error) {
			return t.TempDir(), nil
		},
		PrepareIdentity: func(_ context.Context, token, path string, _ setup.IdentityOperator) error {
			events = append(events, "prepare-identity")
			if token != "test-token" || path != identity {
				t.Fatalf("prepare identity token=%q path=%q", token, path)
			}
			return nil
		},
		RunVast: func(_ context.Context, _ string, selectedRecipe recipe.Recipe, options setup.Options, deps setup.Dependencies) (setup.Result, error) {
			gotOptions = options
			if selectedRecipe.ID != "qwen-studio" || options.AutoSelectOffer || options.OfferLimit != 100 {
				t.Fatalf("recipe=%q auto-select=%v", selectedRecipe.ID, options.AutoSelectOffer)
			}
			if deps.PrepareIdentity == nil || deps.ValidateIdentity == nil {
				t.Fatal("production dependencies omitted identity preparation or validation")
			}
			events = append(events, "run-vast")
			if err := deps.PrepareIdentity(context.Background()); err != nil {
				return setup.Result{}, err
			}
			if err := deps.ValidateIdentity(options.IdentityFile); err != nil {
				return setup.Result{}, err
			}
			validated = options.IdentityFile
			return setup.Result{InstanceID: 1}, nil
		},
	})
	var output bytes.Buffer
	if err := app.setup(strings.NewReader(""), &output, "config.toml", "ubuntu"); err != nil {
		t.Fatal(err)
	}
	if gotOptions.IdentityFile != identity || validated != identity {
		t.Fatalf("identity option=%q validated=%q want %q", gotOptions.IdentityFile, validated, identity)
	}
	if !reflect.DeepEqual(events, []string{"run-vast", "prepare-identity"}) {
		t.Fatalf("events=%v", events)
	}
}

func TestProductionSetupLoadsAndLaunchesSoloProfile(t *testing.T) {
	identity := filepath.Join(t.TempDir(), "identity")
	var selected recipe.Recipe
	app := productionApplicationWith(productionDependencies{
		Prompter: vastSetupPrompter{identity: identity, workload: "qwen-solo-rtx5090"},
		Getenv:   func(string) string { return "test-token" },
		HomeDir:  func() (string, error) { return t.TempDir(), nil },
		RunVast: func(_ context.Context, _ string, candidate recipe.Recipe, _ setup.Options, _ setup.Dependencies) (setup.Result, error) {
			selected = candidate
			return setup.Result{InstanceID: 1, ConfigPath: "config.toml"}, nil
		},
	})
	if err := app.setup(strings.NewReader(""), io.Discard, "config.toml", "ubuntu"); err != nil {
		t.Fatal(err)
	}
	if selected.ID != "qwen-solo-rtx5090" || selected.Profile.Status != "experimental" || selected.Requirements.GPUModel != "RTX 5090" || !selected.Requirements.StrictGPU {
		t.Fatalf("selected profile=%#v", selected)
	}
}

func TestRunWithSetupWritesPrivateRouteConfig(t *testing.T) {
	dir := t.TempDir()
	identity := filepath.Join(dir, "identity")
	knownHosts := filepath.Join(dir, "known_hosts")
	for _, path := range []string{identity, knownHosts} {
		if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	configPath := filepath.Join(dir, "config.toml")
	input := strings.NewReader("")
	var output bytes.Buffer
	app := application{
		setup: func(actualInput io.Reader, actualOutput io.Writer, path, user string) error {
			return cli.Setup(context.Background(), actualInput, actualOutput, path, user, cli.SetupDependencies{
				Prompter: manualSetupPrompter{route: cli.ManualRoute{
					Host:           "gpu.example",
					Port:           22,
					User:           "ubuntu",
					IdentityFile:   identity,
					KnownHostsFile: knownHosts,
				}},
			})
		},
	}
	if err := runWith([]string{"setup"}, input, &output, configPath, "ubuntu", app); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SSH.Host != "gpu.example" || cfg.SSH.User != "ubuntu" {
		t.Fatalf("unexpected config: %#v", cfg.SSH)
	}
}

func TestProductionManualSetupDoesNotLoadVastWorkloads(t *testing.T) {
	dir := t.TempDir()
	identity := filepath.Join(dir, "identity")
	knownHosts := filepath.Join(dir, "known_hosts")
	if err := os.WriteFile(identity, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(knownHosts, []byte("host ssh-ed25519 QUFBQQ=="), 0o600); err != nil {
		t.Fatal(err)
	}
	loadCalls := 0
	app := productionApplicationWith(productionDependencies{
		Prompter: manualSetupPrompter{route: cli.ManualRoute{
			Host:           "gpu.example",
			Port:           22,
			User:           "alice",
			IdentityFile:   identity,
			KnownHostsFile: knownHosts,
		}},
		LoadRecipes: func() ([]recipe.Recipe, error) {
			loadCalls++
			return nil, errors.New("Vast recipes unavailable")
		},
	})

	if err := app.setup(strings.NewReader(""), io.Discard, filepath.Join(dir, "config.toml"), "alice"); err != nil {
		t.Fatal(err)
	}
	if loadCalls != 0 {
		t.Fatalf("recipe load calls=%d", loadCalls)
	}
}

func TestFallbackSetupDeclinedReplacementNeverSavesOrCreates(t *testing.T) {
	for _, kind := range []string{"valid", "invalid", "dangling symlink"} {
		for _, provider := range []string{"manual", "vast"} {
			for _, answer := range []string{"no\n", "", "yes"} {
				t.Run(kind+"/"+provider+"/"+answer, func(t *testing.T) {
					dir := t.TempDir()
					path := filepath.Join(dir, "config.toml")
					key, hosts := filepath.Join(dir, "key"), filepath.Join(dir, "hosts")
					for _, file := range []string{key, hosts} {
						if err := os.WriteFile(file, []byte("fixture"), 0600); err != nil {
							t.Fatal(err)
						}
					}
					switch kind {
					case "valid":
						if err := config.Save(path, config.Studio("original.example", 22, "original", key, hosts)); err != nil {
							t.Fatal(err)
						}
					case "invalid":
						if err := os.WriteFile(path, []byte("not valid TOML"), 0600); err != nil {
							t.Fatal(err)
						}
					case "dangling symlink":
						if err := os.Symlink(filepath.Join(dir, "missing"), path); err != nil {
							t.Fatal(err)
						}
					}
					before, _ := os.ReadFile(path)
					linkBefore, _ := os.Readlink(path)
					var prompter cli.SetupPrompter = manualSetupPrompter{route: cli.ManualRoute{Host: "replacement.example", Port: 22, User: "new-user", IdentityFile: key, KnownHostsFile: hosts}}
					if provider == "vast" {
						prompter = vastSetupPrompter{identity: key}
					}
					creates := 0
					app := productionApplicationWith(productionDependencies{Prompter: prompter, Getenv: func(name string) string {
						if name == "ACCESSIBLE" {
							return "1"
						}
						if name == "VAST_API_KEY" {
							return "test-token"
						}
						return ""
					}, HomeDir: func() (string, error) { return dir, nil }, RunVast: func(context.Context, string, recipe.Recipe, setup.Options, setup.Dependencies) (setup.Result, error) {
						creates++
						return setup.Result{InstanceID: 12}, nil
					}})
					var output bytes.Buffer
					err := app.setup(strings.NewReader(answer), &output, path, "alice")
					if answer == "no\n" && err != nil {
						t.Fatal(err)
					}
					if answer != "no\n" && !errors.Is(err, io.EOF) {
						t.Fatalf("incomplete consent error=%v want EOF", err)
					}
					after, _ := os.ReadFile(path)
					linkAfter, _ := os.Readlink(path)
					if creates != 0 || !bytes.Equal(before, after) || linkBefore != linkAfter {
						t.Fatalf("declined replacement changed state: creates=%d before=%q after=%q", creates, before, after)
					}
					if !strings.Contains(output.String(), "Replace existing configuration") {
						t.Fatalf("replacement was not requested: %s", output.String())
					}
				})
			}
		}
	}
}

func TestFallbackSetupApprovedReplacementRetainsRemainingInput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	key, hosts := filepath.Join(dir, "key"), filepath.Join(dir, "hosts")
	for _, file := range []string{key, hosts} {
		if err := os.WriteFile(file, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := config.Save(path, config.Studio("original.example", 22, "original", key, hosts)); err != nil {
		t.Fatal(err)
	}
	app := productionApplicationWith(productionDependencies{Getenv: func(name string) string {
		if name == "ACCESSIBLE" {
			return "1"
		}
		return ""
	}})
	input := strings.NewReader("yes\n2\nreplacement.example\n\n\n" + key + "\n" + hosts + "\n")
	if err := app.setup(input, io.Discard, path, "alice"); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil || cfg.SSH.Host != "replacement.example" || cfg.SSH.User != "alice" {
		t.Fatalf("replacement route=%+v err=%v", cfg.SSH, err)
	}
}
