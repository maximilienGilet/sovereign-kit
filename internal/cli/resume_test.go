package cli

import (
	"bytes"
	"context"
	"encoding/json"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type resumeTestPrompter struct {
	fakeSetupPrompter
	approve bool
	id      int
}

func TestAccessibleResumeCanDestroyWithoutLaunching(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cp := setup.Checkpoint{Version: 1, InstanceID: 987, Recipe: testRecipe(), IdentityFile: "/tmp/identity", KnownHostsDir: "/tmp/hosts", Phase: "created"}
	data, _ := json.Marshal(cp)
	if err := os.WriteFile(setup.CheckpointPath(path), data, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		input     string
		destroyed int
	}{{"2\nyes\n", 1}, {"2\nno\n", 0}} {
		var output bytes.Buffer
		p := NewAccessiblePrompter(strings.NewReader(tc.input), &output)
		calls := 0
		deps := SetupDependencies{Prompter: p, Getenv: func(string) string { return "token" }, RunVast: func(_ context.Context, _ string, _ string, w VastWorkload, operator setup.Operator) (setup.Result, error) {
			if !w.Resume || !w.RecoveryOnly {
				t.Fatal("destroy started provisioning")
			}
			operator.(setup.RecoveryObserver).InstanceCreated(setup.InstanceRecovery{InstanceID: 987, Destroy: func(context.Context) error { calls++; return nil }})
			return setup.Result{InstanceID: 987}, nil
		}}
		_ = ResumeSetup(context.Background(), &output, path, deps, 0, false)
		if calls != tc.destroyed {
			t.Fatalf("destroy count %d", calls)
		}
	}
}

func (p *resumeTestPrompter) ConfirmResume(_ context.Context, id int, _ recipe.Recipe, _ string) (bool, error) {
	p.id = id
	return p.approve, nil
}

func TestResumeUsesPinnedCheckpointWithoutOfferOrRecipeSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cp := setup.Checkpoint{Version: 1, InstanceID: 987, Recipe: testRecipe(), IdentityFile: "/tmp/identity", KnownHostsDir: "/tmp/hosts", Phase: "created"}
	data, _ := json.Marshal(cp)
	if err := os.WriteFile(setup.CheckpointPath(path), data, 0600); err != nil {
		t.Fatal(err)
	}
	p := &resumeTestPrompter{approve: true}
	calls := 0
	store := FileVastCredentials{Path: path + ".vast-api-key"}
	if err := store.Save("saved-resume-token"); err != nil {
		t.Fatal(err)
	}
	deps := SetupDependencies{Prompter: p, Credentials: store, Getenv: func(string) string { return "" }, RunVast: func(_ context.Context, token, identity string, w VastWorkload, _ setup.Operator) (setup.Result, error) {
		calls++
		if token != "saved-resume-token" {
			t.Fatal("resume did not load saved credential")
		}
		if !w.Resume || w.ResumeInstanceID != 0 || identity != "/tmp/identity" || w.Recipe.ID != cp.Recipe.ID {
			t.Fatalf("resume changed checkpoint: %+v %s", w, identity)
		}
		return setup.Result{InstanceID: 987}, nil
	}}
	if err := ResumeSetup(context.Background(), &bytes.Buffer{}, path, deps, 0, false); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || p.id != 987 {
		t.Fatal("resume did not explicitly confirm existing instance")
	}
	if p.apiKeyCalls != 0 {
		t.Fatal("resume asked for saved API key")
	}
	p.approve = false
	if err := ResumeSetup(context.Background(), &bytes.Buffer{}, path, deps, 0, false); err == nil {
		t.Fatal("decline did not stop resume")
	}
	if calls != 1 {
		t.Fatal("decline started work")
	}
}

func TestMalformedCheckpointBlocksStartupSetup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(setup.CheckpointPath(path), []byte("{broken"), 0600)
	m := newApplication(context.Background(), path, "root", "setup", ApplicationDependencies{})
	_, cmd := m.Update(applicationBegin{})
	if cmd != nil || m.screen != "error" || !m.locked || m.session != nil {
		t.Fatal("invalid checkpoint allowed setup")
	}
}

func TestResumeWithoutCheckpointOffersLegacyRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := newApplication(context.Background(), path, "root", "resume", ApplicationDependencies{})
	_, cmd := m.Update(applicationBegin{})
	if cmd != nil || m.screen != "resume" || m.instanceID != 0 {
		t.Fatalf("missing checkpoint blocked recovery: %s", m.screen)
	}
	if strings.Contains(m.View(), "Creation outcome uncertain") {
		t.Fatal("legacy recovery incorrectly implies paid creation")
	}
}

func TestHomeDoesNotOfferManualRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := newApplication(context.Background(), path, "root", "home", ApplicationDependencies{Setup: SetupDependencies{RunVast: func(context.Context, string, string, VastWorkload, setup.Operator) (setup.Result, error) {
		t.Error("provisioning started before explicit recovery confirmation")
		return setup.Result{}, nil
	}}})
	t.Cleanup(m.cleanup)
	m.Update(applicationBegin{})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if m.screen != "home" || m.session != nil {
		t.Fatal("removed recovery shortcut still opens recovery")
	}
	for _, configured := range []bool{false, true} {
		m.configured = configured
		if strings.Contains(strings.ToLower(m.View()), "recover") {
			t.Fatal("home still offers manual recovery")
		}
	}
}

func TestPendingCheckpointTakesPriorityWithoutStartingWork(t *testing.T) {
	for _, entry := range []string{"home", "setup", "start", "resume"} {
		path := filepath.Join(t.TempDir(), "config.toml")
		cp := setup.Checkpoint{Version: 1, InstanceID: 987, Recipe: testRecipe(), IdentityFile: "/tmp/identity", KnownHostsDir: "/tmp/hosts", Phase: "created"}
		data, _ := json.Marshal(cp)
		if err := os.WriteFile(setup.CheckpointPath(path), data, 0600); err != nil {
			t.Fatal(err)
		}
		m := newApplication(context.Background(), path, "root", entry, ApplicationDependencies{})
		_, cmd := m.Update(applicationBegin{})
		if cmd != nil || m.screen != "resume" || m.instanceID != 987 || m.session != nil {
			t.Fatalf("%s bypassed resume: %s", entry, m.screen)
		}
	}
}
