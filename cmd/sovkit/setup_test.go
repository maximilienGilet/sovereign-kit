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
