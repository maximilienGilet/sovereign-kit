package clientprofile

import (
	"context"
	"encoding/json"
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
	s.Run = func(ctx context.Context, command string, args, env []string) error {
		if command != "pi" || len(args) != 2 || args[0] != "install" {
			t.Fatalf("unexpected install %s %v", command, args)
		}
		var root string
		for _, v := range env {
			if strings.HasPrefix(v, "PI_CODING_AGENT_DIR=") {
				root = strings.TrimPrefix(v, "PI_CODING_AGENT_DIR=")
			}
		}
		name, version := "pi-subagents", "0.62.0"
		if strings.Contains(args[1], "oh-my-pi") {
			name, version = "oh-my-pi", "0.2.0"
		}
		dir := filepath.Join(root, "npm", "node_modules", name)
		os.MkdirAll(filepath.Join(dir, "dist"), 0700)
		os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"`+name+`","version":"`+version+`","pi":{"extensions":["./dist/extension.js"]}}`), 0600)
		os.WriteFile(filepath.Join(dir, "dist", "extension.js"), []byte("// fixture"), 0600)
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
	raw, _ := os.ReadFile(settings)
	var v map[string]any
	json.Unmarshal(raw, &v)
	v["theme"] = "my-theme"
	raw, _ = json.Marshal(v)
	os.WriteFile(settings, raw, 0600)
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
	raw, _ = os.ReadFile(settings)
	json.Unmarshal(raw, &v)
	if v["theme"] != "my-theme" || v["defaultModel"] != "owner/updated" {
		t.Fatalf("settings %s", raw)
	}
	for _, root := range []string{target.Path} {
		raw, err = os.ReadFile(filepath.Join(root, "auth.json"))
		if err != nil || string(raw) != "secret fixture" {
			t.Fatal("lost unrelated credentials")
		}
	}
	old, err := readObject(filepath.Join(got.Backup, "settings.json"))
	if err != nil || old["defaultModel"] != "owner/solo" {
		t.Fatal("previous configuration not recoverable")
	}
}
func TestFailedInstallAndChangedOrUnsafeProfileNeverOverwrite(t *testing.T) {
	s, target, model := fixture(t)
	ctx := context.Background()
	before := s.Inspect(ctx, target, model)
	s.Run = func(context.Context, string, []string, []string) error { return errors.New("installer failed") }
	got, err := s.Install(ctx, target, model, before)
	if err == nil || got.State == Ready {
		t.Fatal("failure unlocked readiness")
	}
	if _, err := os.Stat(target.Path); !os.IsNotExist(err) {
		t.Fatal("failed install published profile")
	}
	os.MkdirAll(target.Path, 0700)
	os.WriteFile(filepath.Join(target.Path, "settings.json"), []byte("{"), 0600)
	if got = s.Inspect(ctx, target, model); got.State != Unreadable {
		t.Fatalf("bad JSON %#v", got)
	}
	if _, err = s.Install(ctx, target, model, before); err == nil {
		t.Fatal("changed inspection accepted")
	}
	other := t.TempDir()
	os.Remove(filepath.Join(target.Path, "settings.json"))
	os.Symlink(other, filepath.Join(target.Path, "models.json"))
	if got = s.Inspect(ctx, target, model); got.State != Unreadable {
		t.Fatalf("symlink accepted %#v", got)
	}
}
func TestMissingLimitsAndGlobalOverrideAreRejected(t *testing.T) {
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
	if _, err := s.Resolve(Pi); err == nil {
		t.Fatal("global Pi profile allowed")
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

func TestFailedDependencyUpdateLeavesExistingProfileUntouched(t *testing.T) {
	s, target, e := fixture(t)
	ctx := context.Background()
	if _, err := s.Install(ctx, target, e, s.Inspect(ctx, target, e)); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(target.Path, "settings.json")
	original, _ := os.ReadFile(settings)
	os.Remove(filepath.Join(target.Path, "npm/node_modules/oh-my-pi/dist/extension.js"))
	s.Run = func(context.Context, string, []string, []string) error { return errors.New("network interrupted") }
	if _, err := s.Install(ctx, target, e, s.Inspect(ctx, target, e)); err == nil {
		t.Fatal("failed update succeeded")
	}
	after, _ := os.ReadFile(settings)
	if string(after) != string(original) {
		t.Fatal("failed package update changed existing configuration")
	}
}
