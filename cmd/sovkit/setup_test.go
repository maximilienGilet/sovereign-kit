package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maximilienGilet/sovereign-kit/internal/cli"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
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

func (p manualSetupPrompter) ManualRoute(context.Context, string) (cli.ManualRoute, error) {
	return p.route, nil
}

func (manualSetupPrompter) VastIdentity(context.Context, string) (string, error) {
	return "", nil
}

type vastSetupPrompter struct {
	identity string
}

func (p vastSetupPrompter) SelectProvider(context.Context) (string, error) {
	return "vast", nil
}

func (vastSetupPrompter) ManualRoute(context.Context, string) (cli.ManualRoute, error) {
	return cli.ManualRoute{}, nil
}

func (p vastSetupPrompter) VastIdentity(context.Context, string) (string, error) {
	return p.identity, nil
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
	var validated string
	app := productionApplicationWith(productionDependencies{
		Prompter: vastSetupPrompter{identity: identity},
		Getenv: func(string) string {
			return "test-token"
		},
		HomeDir: func() (string, error) {
			return t.TempDir(), nil
		},
		RunVast: func(_ context.Context, _ string, _ recipe.Recipe, options setup.Options, deps setup.Dependencies) (setup.Result, error) {
			gotOptions = options
			if deps.ValidateIdentity == nil {
				t.Fatal("production dependencies omitted identity validation")
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
