package clientprofile

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOMPResolvesActiveNamedProfileWithoutLosingItsCustomization(t *testing.T) {
	for _, tc := range []struct {
		name              string
		env               map[string]string
		profile, relative string
	}{
		{"primary profile", map[string]string{"OMP_PROFILE": "work", "PI_CODING_AGENT_DIR": "/ignored/agent"}, "work", ".omp/profiles/work/agent"},
		{"legacy profile", map[string]string{"PI_PROFILE": "personal"}, "personal", ".omp/profiles/personal/agent"},
		{"primary precedence", map[string]string{"OMP_PROFILE": "work", "PI_PROFILE": "personal"}, "work", ".omp/profiles/work/agent"},
		{"custom config root", map[string]string{"OMP_PROFILE": "work", "PI_CONFIG_DIR": ".my-omp"}, "work", ".my-omp/profiles/work/agent"},
		{"explicit default", map[string]string{"OMP_PROFILE": "default", "PI_PROFILE": "personal"}, "", ".omp/agent"},
		{"explicit empty", map[string]string{"OMP_PROFILE": "", "PI_PROFILE": "personal"}, "", ".omp/agent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, e := fixture(t)
			s.Getenv = func(k string) string { return tc.env[k] }
			s.LookupEnv = func(k string) (string, bool) { v, ok := tc.env[k]; return v, ok }
			dir := filepath.Join(s.Home, tc.relative)
			os.MkdirAll(filepath.Join(dir, "skills"), 0700)
			settings := filepath.Join(dir, "config.yml")
			os.WriteFile(settings, []byte("model: keep/my-model\n"), 0600)
			target, err := s.Resolve(OMP)
			if err != nil || target.Path != filepath.Join(dir, "models.yml") {
				t.Fatalf("lost normal profile: %#v %v", target, err)
			}
			got, err := s.Install(context.Background(), target, e, s.Inspect(context.Background(), target, e))
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := os.ReadFile(settings)
			if string(raw) != "model: keep/my-model\n" {
				t.Fatal("profile settings modified")
			}
			shell := "omp() { printf '%s\\n' \"$OMP_PROFILE\" \"$PI_PROFILE\" \"$PI_CODING_AGENT_DIR\" \"$PI_CONFIG_DIR\" \"$@\"; }; " + got.Command
			cmd := exec.Command("sh", "-c", shell)
			cmd.Env = append(os.Environ(), "OMP_PROFILE=other", "PI_PROFILE=other", "PI_CONFIG_DIR=.other")
			output, err := cmd.CombinedOutput()
			configDir := tc.env["PI_CONFIG_DIR"]
			if configDir == "" {
				configDir = ".omp"
			}
			want := tc.profile + "\n" + tc.profile + "\n" + dir + "\n" + configDir + "\n--model\nsovereign-qwen/owner/solo\n"
			if err != nil || string(output) != want {
				t.Fatalf("launch lost selected profile: %q want %q %v", output, want, err)
			}
		})
	}
}

func TestOMPRejectsUnsafeNamedProfile(t *testing.T) {
	for _, name := range []string{"../escape", "work/other", "work.", "CON", "UPPERCASE"} {
		s, _, _ := fixture(t)
		s.Getenv = func(k string) string {
			if k == "OMP_PROFILE" {
				return name
			}
			return ""
		}
		if _, err := s.Resolve(OMP); err == nil {
			t.Fatalf("unsafe OMP profile accepted: %q", name)
		}
	}
}

func TestOMPYAMLUpdatePreservesOtherModelsAndNestedCustomizations(t *testing.T) {
	s, _, e := fixture(t)
	target, _ := s.Resolve(OMP)
	os.MkdirAll(filepath.Dir(target.Path), 0700)
	original := `providers:
  sovereign-qwen:
    headers:
      X-Custom: preserved
    compat:
      customFlag: true
    models:
      # first model stays in place
      - id: owner/other
        contextWindow: 12345
      # selected model keeps metadata
      - id: owner/solo
        contextWindow: 1000
        maxTokens: 100
        custom:
          list: [one, two]
      # trailing model must not disappear
      - id: owner/trailing
        contextWindow: 67890
`
	os.WriteFile(target.Path, []byte(original), 0600)
	ctx := context.Background()
	if _, err := s.Install(ctx, target, e, s.Inspect(ctx, target, e)); err != nil {
		t.Fatal(err)
	}
	config, err := readObject(target.Path)
	if err != nil {
		t.Fatal(err)
	}
	p := config["providers"].(map[string]any)["sovereign-qwen"].(map[string]any)
	models := p["models"].([]any)
	if len(models) != 3 || models[0].(map[string]any)["id"] != "owner/other" || models[2].(map[string]any)["id"] != "owner/trailing" {
		t.Fatalf("model sequence changed: %#v", models)
	}
	selected := models[1].(map[string]any)
	if selected["contextWindow"] != float64(32768) || selected["custom"].(map[string]any)["list"].([]any)[1] != "two" {
		t.Fatalf("selected customization lost: %#v", selected)
	}
	if p["headers"].(map[string]any)["X-Custom"] != "preserved" || p["compat"].(map[string]any)["customFlag"] != true {
		t.Fatal("nested metadata lost")
	}
	raw, _ := os.ReadFile(target.Path)
	for _, comment := range []string{"# first model stays in place", "# selected model keeps metadata", "# trailing model must not disappear"} {
		if !strings.Contains(string(raw), comment) {
			t.Fatalf("comment lost: %s", comment)
		}
	}
}

func TestOMPCustomProviderPreservesYAMLAndNormalSettings(t *testing.T) {
	s, _, e := fixture(t)
	target, err := s.Resolve(Integration("Oh My Pi (OMP)"))
	if err != nil {
		t.Fatal(err)
	}
	if target.Path != filepath.Join(s.Home, ".omp/agent/models.yml") {
		t.Fatalf("wrong OMP config %s", target.Path)
	}
	dir := filepath.Dir(target.Path)
	os.MkdirAll(filepath.Join(dir, "skills/custom"), 0700)
	files := map[string]string{"config.yml": "model: anthropic/keep\n# private settings\n", "settings.json": "legacy settings", "auth.json": "private auth", "skills/custom/SKILL.md": "custom skill"}
	for name, raw := range files {
		os.WriteFile(filepath.Join(dir, name), []byte(raw), 0600)
	}
	original := "# preserve this provider comment\nproviders:\n  existing:\n    apiKey: keep\n    models:\n      - id: old\n        contextWindow: 12345\n"
	os.WriteFile(target.Path, []byte(original), 0600)
	got, err := s.Install(context.Background(), target, e, s.Inspect(context.Background(), target, e))
	if err != nil || got.State != Ready {
		t.Fatalf("install %#v %v", got, err)
	}
	raw, _ := os.ReadFile(target.Path)
	if !strings.Contains(string(raw), "# preserve this provider comment") || !strings.Contains(string(raw), "existing:") {
		t.Fatalf("YAML data lost: %s", raw)
	}
	cfg, err := readObject(target.Path)
	if err != nil {
		t.Fatal(err)
	}
	p := cfg["providers"].(map[string]any)
	if p["existing"].(map[string]any)["apiKey"] != "keep" || p["sovereign-qwen"].(map[string]any)["api"] != "openai-completions" {
		t.Fatalf("providers %#v", p)
	}
	for name, want := range files {
		raw, _ := os.ReadFile(filepath.Join(dir, name))
		if string(raw) != want {
			t.Fatalf("modified %s", name)
		}
	}
	backup, _ := os.ReadFile(filepath.Join(got.Backup, "models.yml"))
	if string(backup) != original {
		t.Fatal("backup changed")
	}
	shell := "omp() { printf '%s\\n' \"$OMP_PROFILE\" \"$PI_PROFILE\" \"$PI_CODING_AGENT_DIR\" \"$@\"; }; " + got.Command
	cmd := exec.Command("sh", "-c", shell)
	cmd.Env = append(os.Environ(), "OMP_PROFILE=isolated", "PI_PROFILE=isolated")
	output, err := cmd.CombinedOutput()
	want := "\n\n" + dir + "\n--model\nsovereign-qwen/owner/solo\n"
	if err != nil || string(output) != want {
		t.Fatalf("OMP selection %q want %q %v", output, want, err)
	}
	again, err := s.Install(context.Background(), target, e, s.Inspect(context.Background(), target, e))
	if err != nil || again.Backup != "" {
		t.Fatalf("not idempotent %#v %v", again, err)
	}
}

func TestOMPPreservesExistingAlternativeConfigAndRejectsMalformedYAML(t *testing.T) {
	for _, name := range []string{"models.yaml", "models.json"} {
		t.Run(name, func(t *testing.T) {
			s, _, e := fixture(t)
			dir := filepath.Join(s.Home, ".omp/agent")
			os.MkdirAll(dir, 0700)
			path := filepath.Join(dir, name)
			os.WriteFile(path, []byte(`{"providers":{"existing":{"apiKey":"keep"}}}`), 0600)
			target, err := s.Resolve(Integration("Oh My Pi (OMP)"))
			if err != nil || target.Path != path {
				t.Fatalf("resolution %#v %v", target, err)
			}
			if _, err := s.Install(context.Background(), target, e, s.Inspect(context.Background(), target, e)); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dir, "models.yml")); !os.IsNotExist(err) {
				t.Fatal("shadowed existing alternate config")
			}
		})
	}
	s, _, e := fixture(t)
	target, _ := s.Resolve(Integration("Oh My Pi (OMP)"))
	os.MkdirAll(filepath.Dir(target.Path), 0700)
	for _, raw := range []string{"providers: [broken", "providers:\n  duplicate: {}\n  duplicate: {}\n", "providers: {}\n---\nproviders: {}\n"} {
		os.WriteFile(target.Path, []byte(raw), 0600)
		got := s.Inspect(context.Background(), target, e)
		if got.State != Unreadable {
			t.Fatalf("unsafe YAML accepted: %#v", got)
		}
	}
}
