package clientprofile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) (Service, Target, Endpoint) {
	t.Helper()
	home := t.TempDir()
	home, _ = filepath.EvalSymlinks(home)
	s := Service{Home: home, Getenv: func(string) string { return "" }, LookPath: func(name string) (string, error) { return "/bin/" + name, nil }}
	target, err := s.Resolve(Pi)
	if err != nil {
		t.Fatal(err)
	}
	s.Run = func(context.Context, string, []string, []string) error {
		t.Fatal("custom provider must not install packages")
		return nil
	}
	return s, target, Endpoint{BaseURL: "http://127.0.0.1:30000/v1", Metadata: Metadata{ID: "owner/solo", ContextWindow: 32768, MaxTokens: 4096}}
}
func TestInspectionIsReadOnlyAndInstallVerifiesPreservingData(t *testing.T) {
	s, target, model := fixture(t)
	ctx := context.Background()
	before := s.Inspect(ctx, target, model)
	if before.State != Absent {
		t.Fatalf("%#v", before)
	}
	if _, err := os.Stat(target.Path); !os.IsNotExist(err) {
		t.Fatal("inspection wrote profile")
	}
	got, err := s.Install(ctx, target, model, before)
	if err != nil || got.State != Ready {
		t.Fatalf("install %#v %v", got, err)
	}
	settings := filepath.Join(target.Path, "settings.json")
	os.WriteFile(settings, []byte(`{"theme":"my-theme","defaultModel":"keep"}`), 0600)
	os.WriteFile(filepath.Join(target.Path, "auth.json"), []byte("secret fixture"), 0600)
	model.ID = "owner/updated"
	before = s.Inspect(ctx, target, model)
	if before.State != Different {
		t.Fatalf("%#v", before)
	}
	got, err = s.Install(ctx, target, model, before)
	if err != nil || got.State != Ready || got.Backup == "" {
		t.Fatalf("update %#v %v", got, err)
	}
	raw, _ := os.ReadFile(settings)
	if string(raw) != `{"theme":"my-theme","defaultModel":"keep"}` {
		t.Fatalf("settings %s", raw)
	}
	for _, root := range []string{target.Path} {
		raw, err = os.ReadFile(filepath.Join(root, "auth.json"))
		if err != nil || string(raw) != "secret fixture" {
			t.Fatal("lost unrelated credentials")
		}
	}
	old, err := readObject(filepath.Join(got.Backup, "models.json"))
	if err != nil || old["providers"].(map[string]any)[provider].(map[string]any)["models"].([]any)[0].(map[string]any)["id"] != "owner/solo" {
		t.Fatal("previous configuration not recoverable")
	}
}
func TestFailedInstallAndChangedOrUnsafeProfileNeverOverwrite(t *testing.T) {
	s, target, model := fixture(t)
	ctx := context.Background()
	before := s.Inspect(ctx, target, model)
	s.LookPath = func(string) (string, error) { return "", errors.New("CLI missing") }
	got, err := s.Install(ctx, target, model, before)
	if err == nil || got.State == Ready {
		t.Fatal("failure unlocked readiness")
	}
	if _, err := os.Stat(target.Path); !os.IsNotExist(err) {
		t.Fatal("failed install published profile")
	}
	os.MkdirAll(target.Path, 0700)
	os.WriteFile(filepath.Join(target.Path, "models.json"), []byte("{"), 0600)
	if got = s.Inspect(ctx, target, model); got.State != Unreadable {
		t.Fatalf("bad JSON %#v", got)
	}
	if _, err = s.Install(ctx, target, model, before); err == nil {
		t.Fatal("changed inspection accepted")
	}
	other := t.TempDir()
	os.Remove(filepath.Join(target.Path, "models.json"))
	os.Symlink(other, filepath.Join(target.Path, "models.json"))
	if got = s.Inspect(ctx, target, model); got.State != Unreadable {
		t.Fatalf("symlink accepted %#v", got)
	}
}
func TestMissingLimitsRejectedAndNormalConfigOverrideAccepted(t *testing.T) {
	s, target, model := fixture(t)
	model.ContextWindow = 0
	if _, err := s.Install(context.Background(), target, model, s.Inspect(context.Background(), target, model)); err == nil {
		t.Fatal("missing limits installed")
	}
	s.Getenv = func(k string) string {
		if k == "PI_CODING_AGENT_DIR" {
			return filepath.Join(s.Home, ".pi", "agent")
		}
		return ""
	}
	if got, err := s.Resolve(Pi); err != nil || got.Path != filepath.Join(s.Home, ".pi", "agent") {
		t.Fatalf("normal Pi config rejected: %#v %v", got, err)
	}
}

func TestOpenCodeUpdatesOnlyItsConfigAndUsesQuotedInlineCommand(t *testing.T) {
	s, _, e := fixture(t)
	s.Getenv = func(k string) string {
		if k == "SOVEREIGN_OPENCODE_CONFIG" {
			return filepath.Join(s.Home, "profile with ' quote", "sovereign.json")
		}
		return ""
	}
	target, err := s.Resolve(OpenCode)
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Dir(target.Path), 0700)
	os.WriteFile(target.Path, []byte(`{"theme":"keep-me","model":"old/model"}`), 0600)
	os.WriteFile(filepath.Join(filepath.Dir(target.Path), "auth.json"), []byte("keep credential"), 0600)
	s.Run = func(context.Context, string, []string, []string) error {
		t.Fatal("OpenCode config update ran package command")
		return nil
	}
	before := s.Inspect(context.Background(), target, e)
	got, err := s.Install(context.Background(), target, e, before)
	if err != nil || got.State != Ready {
		t.Fatalf("OpenCode update %v %#v", err, got)
	}
	cfg, _ := readObject(target.Path)
	if cfg["theme"] != "keep-me" || cfg["model"] != "sovereign-qwen/owner/solo" {
		t.Fatalf("config %#v", cfg)
	}
	if !strings.Contains(got.Command, "OPENCODE_CONFIG_CONTENT=") || !strings.Contains(got.Command, "'\"'\"'") {
		t.Fatalf("unsafe or low precedence command %q", got.Command)
	}
	raw, _ := os.ReadFile(filepath.Join(filepath.Dir(target.Path), "auth.json"))
	if string(raw) != "keep credential" {
		t.Fatal("unrelated OpenCode credentials modified")
	}
}

func TestUnavailableCLILeavesExistingConfigurationUntouched(t *testing.T) {
	s, target, e := fixture(t)
	ctx := context.Background()
	if _, err := s.Install(ctx, target, e, s.Inspect(ctx, target, e)); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(target.Path, "models.json")
	original, _ := os.ReadFile(settings)
	s.LookPath = func(string) (string, error) { return "", errors.New("CLI missing") }
	if _, err := s.Install(ctx, target, e, s.Inspect(ctx, target, e)); err == nil {
		t.Fatal("failed update succeeded")
	}
	after, _ := os.ReadFile(settings)
	if string(after) != string(original) {
		t.Fatal("failed package update changed existing configuration")
	}
}
