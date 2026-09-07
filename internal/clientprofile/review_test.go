package clientprofile

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAbsoluteInternalSymlinkCannotReachLiveDataDuringInstall(t *testing.T) {
	s, target, e := fixture(t)
	ctx := context.Background()
	if _, err := s.Install(ctx, target, e, s.Inspect(ctx, target, e)); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(target.Path, "preserve.txt")
	os.WriteFile(live, []byte("original"), 0600)
	os.Remove(filepath.Join(target.Path, "models.json"))
	os.Symlink(live, filepath.Join(target.Path, "models.json"))
	calls := 0
	s.Run = func(_ context.Context, _ string, _ []string, env []string) error {
		calls++
		return nil
	}
	_, err := s.Install(ctx, target, e, s.Inspect(ctx, target, e))
	raw, _ := os.ReadFile(live)
	if err == nil || calls != 0 || string(raw) != "original" {
		t.Fatalf("staged symlink reached live data: calls=%d data=%q err=%v", calls, raw, err)
	}
}
func TestPiUpdatePreservesNestedProviderAndPackageSettings(t *testing.T) {
	s, target, e := fixture(t)
	ctx := context.Background()
	if _, err := s.Install(ctx, target, e, s.Inspect(ctx, target, e)); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(target.Path, "settings.json")
	settings := map[string]any{}
	settings["packages"] = []any{map[string]any{"source": "npm:pi-subagents@0.61.0", "extensions": []any{"*.ts"}, "custom": map[string]any{"keep": true}}, "npm:oh-my-pi@0.2.0"}
	modelsPath := filepath.Join(target.Path, "models.json")
	models, _ := readObject(modelsPath)
	providers := models["providers"].(map[string]any)
	p := providers[provider].(map[string]any)
	p["headers"] = map[string]any{"X-Custom": "retained"}
	p["compat"].(map[string]any)["customFlag"] = true
	p["models"].([]any)[0].(map[string]any)["customMetadata"] = "keep-model"
	for path, v := range map[string]any{settingsPath: settings, modelsPath: models} {
		raw, _ := json.Marshal(v)
		os.WriteFile(path, raw, 0600)
	}
	e.ContextWindow = 65536
	if _, err := s.Install(ctx, target, e, s.Inspect(ctx, target, e)); err != nil {
		t.Fatal(err)
	}
	settings, _ = readObject(settingsPath)
	entry, ok := settings["packages"].([]any)[0].(map[string]any)
	if !ok || entry["source"] != "npm:pi-subagents@0.61.0" || entry["extensions"] == nil || entry["custom"] == nil {
		t.Fatalf("package options lost %#v", settings["packages"])
	}
	models, _ = readObject(modelsPath)
	p = models["providers"].(map[string]any)[provider].(map[string]any)
	if p["headers"] == nil || p["compat"].(map[string]any)["customFlag"] != true || p["models"].([]any)[0].(map[string]any)["customMetadata"] != "keep-model" {
		t.Fatalf("nested provider settings lost %#v", p)
	}
}
func TestOpenCodeUpdatePreservesNestedProviderOptions(t *testing.T) {
	s, _, e := fixture(t)
	target, _ := s.Resolve(OpenCode)
	ctx := context.Background()
	os.MkdirAll(filepath.Dir(target.Path), 0700)
	os.WriteFile(target.Path, []byte(`{"provider":{"sovereign-qwen":{"options":{"headers":{"X-Custom":"keep"},"timeout":42},"custom":true,"models":{"owner/solo":{"custom":true,"limit":{"custom":7}}}}}}`), 0600)
	if _, err := s.Install(ctx, target, e, s.Inspect(ctx, target, e)); err != nil {
		t.Fatal(err)
	}
	cfg, _ := readObject(target.Path)
	p := cfg["provider"].(map[string]any)[provider].(map[string]any)
	options := p["options"].(map[string]any)
	model := p["models"].(map[string]any)[e.ID].(map[string]any)
	if options["headers"] == nil || options["timeout"] != float64(42) || p["custom"] != true || model["custom"] != true || model["limit"].(map[string]any)["custom"] != float64(7) {
		t.Fatalf("nested OpenCode options lost %#v", p)
	}
}
