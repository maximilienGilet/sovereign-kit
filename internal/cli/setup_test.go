package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

type fakeSetupPrompter struct {
	provider       string
	manual         ManualRoute
	identity       string
	identityInput  string
	selectOfferErr error
	events         *[]string
}

func (p *fakeSetupPrompter) SelectProvider(context.Context) (string, error) {
	if p.events != nil {
		*p.events = append(*p.events, "select-provider")
	}
	return p.provider, nil
}

func (p *fakeSetupPrompter) ManualRoute(context.Context, string) (ManualRoute, error) {
	return p.manual, nil
}

func (p *fakeSetupPrompter) VastIdentity(_ context.Context, defaultValue string) (string, error) {
	p.identityInput = defaultValue
	return p.identity, nil
}

func (p *fakeSetupPrompter) SelectOffer(context.Context, []setup.OfferView) (vast.Offer, error) {
	return vast.Offer{}, p.selectOfferErr
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
		manual: ManualRoute{Host: "gpu.example.test", Port: 2222, User: "sovkit", IdentityFile: identity, KnownHostsFile: knownHosts},
	}
	getenvCalled := false
	deps := SetupDependencies{
		Prompter: prompter,
		Getenv: func(string) string {
			getenvCalled = true
			return "should-not-be-read"
		},
		RunVast: func(context.Context, string, string, setup.Operator) (setup.Result, error) {
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

func TestSetupVastRequiresAPIKeyBeforeRunner(t *testing.T) {
	prompter := &fakeSetupPrompter{provider: "vast", identity: "/home/alice/.ssh/id_ed25519"}
	runnerCalled := false
	deps := SetupDependencies{
		Prompter: prompter,
		Getenv: func(key string) string {
			if key != "VAST_API_KEY" {
				t.Fatalf("unexpected environment lookup %q", key)
			}
			return ""
		},
		HomeDir: func() (string, error) {
			t.Fatal("home directory must not be read before API key validation")
			return "", nil
		},
		RunVast: func(context.Context, string, string, setup.Operator) (setup.Result, error) {
			runnerCalled = true
			return setup.Result{}, nil
		},
	}

	err := Setup(context.Background(), bytes.NewBuffer(nil), &bytes.Buffer{}, filepath.Join(t.TempDir(), "config.toml"), "alice", deps)
	if err == nil || !strings.Contains(err.Error(), "Vast API key is required") {
		t.Fatalf("expected missing API key error, got %v", err)
	}
	if runnerCalled {
		t.Fatal("Vast runner was called without an API key")
	}
}

func TestSetupVastDefaultsToExistingEd25519Identity(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	prompter := &fakeSetupPrompter{provider: "vast", identity: "/home/alice/.ssh/id_ed25519"}
	var gotToken, gotIdentity string
	deps := SetupDependencies{
		Prompter: prompter,
		Getenv: func(string) string { return "exact-secret-token" },
		HomeDir: func() (string, error) { return "/home/alice", nil },
		RunVast: func(_ context.Context, token, identity string, _ setup.Operator) (setup.Result, error) {
			gotToken, gotIdentity = token, identity
			return setup.Result{InstanceID: 41, ConfigPath: configPath}, nil
		},
	}
	if err := Setup(context.Background(), bytes.NewBuffer(nil), &bytes.Buffer{}, configPath, "alice", deps); err != nil {
		t.Fatal(err)
	}
	if prompter.identityInput != filepath.Join("/home/alice", ".ssh", "id_ed25519") {
		t.Fatalf("unexpected identity default %q", prompter.identityInput)
	}
	if gotToken != "exact-secret-token" || gotIdentity != filepath.Join("/home/alice", ".ssh", "id_ed25519") {
		t.Fatalf("unexpected runner args token=%q identity=%q", gotToken, gotIdentity)
	}
}
func TestSetupVastPrintsResultAndStartNextStep(t *testing.T) {
	output := &bytes.Buffer{}
	configPath := filepath.Join(t.TempDir(), "custom-config.toml")
	deps := SetupDependencies{
		Prompter: &fakeSetupPrompter{provider: "vast", identity: "/home/alice/.ssh/id_ed25519"},
		Getenv:   func(string) string { return "secret-token" },
		HomeDir:  func() (string, error) { return "/home/alice", nil },
		RunVast: func(context.Context, string, string, setup.Operator) (setup.Result, error) {
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
