package clientprofile

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A regression that writes settings, installs extensions, isolates the default
// target, or replaces another provider must fail this preservation test.
func TestPiCustomProviderPreservesNormalConfiguration(t *testing.T) {
	s, target, e := fixture(t)
	if target.Path != filepath.Join(s.Home, ".pi", "agent") {
		t.Fatalf("normal Pi config not selected: %s", target.Path)
	}
	ctx := context.Background()
	originals := map[string]string{
		"settings.json": "{\n  \"defaultProvider\": \"anthropic\", \"defaultModel\": \"my-model\", \"packages\": [\"npm:oh-my-pi@0.1.0\"], \"custom\": true\n}\n",
		"auth.json":     "private auth", "skills/custom/SKILL.md": "my skill", "extensions/custom.ts": "my extension", "npm/keep.txt": "my packages",
	}
	for name, raw := range originals {
		path := filepath.Join(target.Path, name)
		os.MkdirAll(filepath.Dir(path), 0700)
		os.WriteFile(path, []byte(raw), 0600)
	}
	originalModels := `{"providers":{"existing":{"apiKey":"keep","models":[{"id":"existing-model"}]}},"custom":true}`
	os.WriteFile(filepath.Join(target.Path, "models.json"), []byte(originalModels), 0600)
	legacy := filepath.Join(s.Home, ".pi/profiles/sovereign/agent/settings.json")
	os.MkdirAll(filepath.Dir(legacy), 0700)
	os.WriteFile(legacy, []byte("legacy untouched"), 0600)
	s.Run = func(context.Context, string, []string, []string) error {
		t.Fatal("provider integration attempted package installation")
		return nil
	}
	before := s.Inspect(ctx, target, e)
	if len(before.Paths) != 1 || before.Paths[0] != filepath.Join(target.Path, "models.json") {
		t.Fatalf("unexpected managed paths: %v", before.Paths)
	}
	got, err := s.Install(ctx, target, e, before)
	if err != nil || got.State != Ready {
		t.Fatalf("install: %#v %v", got, err)
	}
	for name, want := range originals {
		raw, err := os.ReadFile(filepath.Join(target.Path, name))
		if err != nil || string(raw) != want {
			t.Fatalf("modified %s: %q %v", name, raw, err)
		}
	}
	raw, _ := os.ReadFile(legacy)
	if string(raw) != "legacy untouched" {
		t.Fatal("legacy profile changed")
	}
	saved, _ := os.ReadFile(filepath.Join(got.Backup, "models.json"))
	if string(saved) != originalModels {
		t.Fatal("backup not byte-for-byte original")
	}
	models, _ := readObject(filepath.Join(target.Path, "models.json"))
	providers := models["providers"].(map[string]any)
	if providers["existing"].(map[string]any)["apiKey"] != "keep" || models["custom"] != true {
		t.Fatal("unrelated provider data lost")
	}
	again, err := s.Install(ctx, target, e, s.Inspect(ctx, target, e))
	if err != nil || again.Backup != "" || again.State != Ready {
		t.Fatalf("non-idempotent update: %#v %v", again, err)
	}
}

func TestProviderUpdateRejectsConcurrentChangesAndRollsBackVerificationFailure(t *testing.T) {
	for _, failure := range []string{"concurrent edit", "final verification"} {
		t.Run(failure, func(t *testing.T) {
			s, target, e := fixture(t)
			ctx := context.Background()
			if _, err := s.Install(ctx, target, e, s.Inspect(ctx, target, e)); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(target.Path, "models.json")
			original, _ := os.ReadFile(path)
			e.ContextWindow = 65536
			confirmed := s.Inspect(ctx, target, e)
			calls := 0
			want := original
			s.LookPath = func(name string) (string, error) {
				calls++
				if failure == "concurrent edit" && calls == 2 {
					want = []byte(`{"providers":{"user-edit":{"apiKey":"do-not-overwrite"}}}`)
					if err := os.WriteFile(path, want, 0600); err != nil {
						t.Fatal(err)
					}
				}
				if failure == "final verification" && calls == 3 {
					return "", errors.New("CLI disappeared")
				}
				return "/bin/" + name, nil
			}
			got, err := s.Install(ctx, target, e, confirmed)
			if err == nil || got.State == Ready {
				t.Fatalf("failed update unlocked launch: %#v %v", got, err)
			}
			after, _ := os.ReadFile(path)
			if string(after) != string(want) {
				t.Fatalf("lost preserved configuration: %s", after)
			}
		})
	}
}

func TestPiLaunchCommandSelectsExactModelWithShellEscaping(t *testing.T) {
	s, _, e := fixture(t)
	custom := filepath.Join(s.Home, "agent with ' quote")
	s.Getenv = func(k string) string {
		if k == "PI_CODING_AGENT_DIR" {
			return custom
		}
		return ""
	}
	target, err := s.Resolve(Pi)
	if err != nil {
		t.Fatal(err)
	}
	e.ID = "owner/model ' literal $(echo unsafe)"
	got, err := s.Install(context.Background(), target, e, s.Inspect(context.Background(), target, e))
	if err != nil {
		t.Fatal(err)
	}
	// Execute the emitted shell with a shell function at the external CLI boundary.
	shell := "pi() { printf '%s\\n' \"$PI_CODING_AGENT_DIR\" \"$@\"; }; " + got.Command
	output, err := exec.Command("sh", "-c", shell).CombinedOutput()
	want := custom + "\n--provider\nsovereign-qwen\n--model\n" + e.ID + "\n"
	if err != nil || string(output) != want {
		t.Fatalf("command args: %q, want %q (%v)", output, want, err)
	}
	if strings.Contains(got.Command, "profiles/sovereign") {
		t.Fatal("launch isolated profile")
	}
}
