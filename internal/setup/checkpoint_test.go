package setup

import (
	"context"
	"errors"
	"fmt"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckpointRecoveryCannotDestroyWhileSetupLocked(t *testing.T) {
	options := testOptions()
	options.CheckpointPath = filepath.Join(t.TempDir(), "pending.json")
	cp := Checkpoint{Version: 1, InstanceID: 987, Recipe: validRecipe(), IdentityFile: "identity", KnownHostsDir: "hosts", Phase: "created"}
	if err := writeCheckpoint(options.CheckpointPath, cp); err != nil {
		t.Fatal(err)
	}
	unlock, err := lockCheckpoint(options.CheckpointPath)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	recovery := newInstanceRecovery(987, &recoveryVastAPI{}, options, &fakeClock{})
	if err := recovery.Destroy(context.Background()); err == nil {
		t.Fatal("destroy ignored provisioning lock")
	}
}

func TestSuccessfulSetupRefreshesRecoveryAfterCheckpointRetirement(t *testing.T) {
	api, operator, _, _, _, _, deps := successfulSetup()
	deps.ServerLauncher = &memoryReconciler{}
	wrapped := &recoveryVastAPI{fakeVastAPI: *api}
	deps.NewAPI = func(string) VastAPI { return wrapped }
	options := testOptions()
	options.CheckpointPath = filepath.Join(t.TempDir(), "pending.json")
	if _, err := RunVast(context.Background(), "token", validRecipe(), options, deps); err != nil {
		t.Fatal(err)
	}
	if err := operator.recovery.Destroy(context.Background()); err != nil {
		t.Fatalf("successful setup left stale recovery checkpoint requirement: %v", err)
	}
}

type memoryReconciler struct {
	running  bool
	launches int
}

func (m *memoryReconciler) Launch(context.Context, config.SSH, recipe.Recipe) error {
	panic("unsafe launch")
}
func (m *memoryReconciler) Reconcile(context.Context, config.SSH, recipe.Recipe) error {
	if !m.running {
		m.running = true
		m.launches++
	}
	return nil
}

func TestResumeAfterSaveFailureReusesDeploymentAndRetiresJournal(t *testing.T) {
	api, _, _, _, _, _, deps := successfulSetup()
	running := vast.Instance{ID: 987, Status: "running", SSHHost: "gpu.example", SSHPort: 22022}
	api.instances = []vast.Instance{running, running, running}
	options := testOptions()
	options.CheckpointPath = filepath.Join(t.TempDir(), "pending.json")
	options.KnownHostsDir = t.TempDir()
	launcher := &memoryReconciler{}
	deps.ServerLauncher = launcher
	deps.TrustStore = FileTrustStore{}
	deps.SaveConfig = func(string, config.Config) error { return errors.New("disk unavailable") }
	if _, err := RunVast(context.Background(), "token", validRecipe(), options, deps); err == nil {
		t.Fatal("expected persistence failure")
	}
	cp, err := ReadCheckpoint(options.CheckpointPath)
	if err != nil || cp.Phase != "launch-intent" {
		t.Fatalf("cp=%+v err=%v", cp, err)
	}
	options.Resume = true
	deps.SaveConfig = func(string, config.Config) error { return nil }
	result, err := RunVast(context.Background(), "token", recipe.Recipe{}, options, deps)
	if err != nil || result.InstanceID != 987 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if launcher.launches != 1 || api.createCalls != 1 || api.searchCalls != 1 {
		t.Fatal("duplicated deployment")
	}
	if _, err := ReadCheckpoint(options.CheckpointPath); !os.IsNotExist(err) {
		t.Fatalf("checkpoint remained: %v", err)
	}
}

func TestResumeRejectsChangedPinnedHostKeys(t *testing.T) {
	dir := t.TempDir()
	options := testOptions()
	options.Resume = true
	options.CheckpointPath = filepath.Join(dir, "pending.json")
	options.KnownHostsDir = dir
	knownHostsPath := filepath.Join(dir, "vast-987_known_hosts")
	oldKeys := []byte("gpu.example ssh-ed25519 AAAA\n")
	if err := os.WriteFile(knownHostsPath, oldKeys, 0600); err != nil {
		t.Fatal(err)
	}
	cp := Checkpoint{
		Version: 1, InstanceID: 987, Recipe: validRecipe(), IdentityFile: options.IdentityFile,
		KnownHostsDir: dir, Phase: "trusted", ApprovedHost: "gpu.example", ApprovedPort: 22022,
		KnownHostsFile: knownHostsPath, HostKeysHash: hostKeysDigest(oldKeys),
	}
	if err := writeCheckpoint(options.CheckpointPath, cp); err != nil {
		t.Fatal(err)
	}
	running := vast.Instance{ID: 987, Status: "running", SSHHost: "gpu.example", SSHPort: 22022}
	api := &fakeVastAPI{instances: []vast.Instance{running, running}}
	trust := &fakeTrustStore{}
	launcher := &fakeLauncher{}
	deps := baseDependencies(api, &fakeOperator{confirmKeys: true}, &fakeScanner{keys: HostKeys{Raw: []byte("gpu.example ssh-ed25519 EVIL\n"), Fingerprints: []string{"SHA256:evil"}}}, trust, launcher, &fakeClock{}, nil)
	_, err := RunVast(context.Background(), "token", recipe.Recipe{}, options, deps)
	if err == nil || !strings.Contains(err.Error(), "host keys changed") {
		t.Fatalf("changed pinned keys accepted: %v", err)
	}
	if trust.calls != 0 || launcher.calls != 0 {
		t.Fatalf("changed keys caused side effects: trust=%d launch=%d", trust.calls, launcher.calls)
	}
}

func TestRecoveryOnlyAllowsOfflineWithoutLaunchAndClearsAfterAbsence(t *testing.T) {
	api, operator, _, _, launcher, _, deps := successfulSetup()
	api.instances = []vast.Instance{{ID: 987, Status: "offline"}}
	wrapped := &recoveryVastAPI{fakeVastAPI: *api}
	deps.NewAPI = func(string) VastAPI { return wrapped }
	options := testOptions()
	options.CheckpointPath = filepath.Join(t.TempDir(), "pending.json")
	options.Resume = true
	options.RecoveryOnly = true
	cp := Checkpoint{Version: 1, InstanceID: 987, Recipe: validRecipe(), IdentityFile: options.IdentityFile, KnownHostsDir: options.KnownHostsDir, Phase: "created"}
	if err := writeCheckpoint(options.CheckpointPath, cp); err != nil {
		t.Fatal(err)
	}
	if _, err := RunVast(context.Background(), "token", recipe.Recipe{}, options, deps); err != nil {
		t.Fatal(err)
	}
	if launcher.calls != 0 || operator.recovery.Destroy == nil {
		t.Fatal("unsafe recovery")
	}
	if err := operator.recovery.Destroy(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadCheckpoint(options.CheckpointPath); !os.IsNotExist(err) {
		t.Fatal("verified absence retained journal")
	}
}

func TestLegacyResumeVerifiesPinnedImageAndNeverCreates(t *testing.T) {
	for _, matches := range []bool{false, true} {
		t.Run(fmt.Sprint(matches), func(t *testing.T) {
			api, _, _, _, _, _, deps := successfulSetup()
			image := "mutable:latest"
			if matches {
				image = validRecipe().Runtime.Image
			}
			api.instances = []vast.Instance{{ID: 987, Status: "offline", Image: image}, {ID: 987, Status: "offline", Image: image}}
			wrapped := &recoveryVastAPI{fakeVastAPI: *api}
			deps.NewAPI = func(string) VastAPI { return wrapped }
			opts := testOptions()
			opts.CheckpointPath = filepath.Join(t.TempDir(), "pending")
			opts.ResumeInstanceID = 987
			opts.RecoveryOnly = true
			_, err := RunVast(context.Background(), "token", validRecipe(), opts, deps)
			if (err == nil) != matches {
				t.Fatalf("image matches=%v err=%v", matches, err)
			}
			if wrapped.createCalls != 0 || wrapped.searchCalls != 0 {
				t.Fatal("legacy resume created")
			}
		})
	}
}

func TestDurablePollingRejectsDifferentInstance(t *testing.T) {
	api, _, scanner, _, _, _, deps := successfulSetup()
	api.instances = []vast.Instance{{ID: 123, Status: "running", SSHHost: "foreign", SSHPort: 22}}
	options := testOptions()
	options.CheckpointPath = filepath.Join(t.TempDir(), "pending")
	_, err := RunVast(context.Background(), "token", validRecipe(), options, deps)
	if err == nil || scanner.calls != 0 {
		t.Fatal("foreign instance reached SSH")
	}
}

func TestCheckpointCreationIntentExistsBeforePaidRequest(t *testing.T) {
	api, _, _, _, _, _, deps := successfulSetup()
	options := testOptions()
	options.CheckpointPath = filepath.Join(t.TempDir(), "pending.json")
	wrapped := &intentAPI{fakeVastAPI: api, t: t, path: options.CheckpointPath}
	deps.NewAPI = func(string) VastAPI { return wrapped }
	_, _ = RunVast(context.Background(), "secret-token", validRecipe(), options, deps)
	data, err := os.ReadFile(options.CheckpointPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret-token") {
		t.Fatal("credential leaked")
	}
}

type intentAPI struct {
	*fakeVastAPI
	t    *testing.T
	path string
}

func (a *intentAPI) CreateInstance(ctx context.Context, id int, req vast.CreateRequest) (int, error) {
	cp, err := ReadCheckpoint(a.path)
	if err != nil || cp.Phase != "create-intent" || cp.InstanceID != 0 {
		a.t.Fatalf("missing prior creation intent: %+v %v", cp, err)
	}
	return a.fakeVastAPI.CreateInstance(ctx, id, req)
}

func TestCheckpointRetainsPaidIDAndBlocksNewCreation(t *testing.T) {
	api, _, scanner, _, _, _, deps := successfulSetup()
	options := testOptions()
	options.CheckpointPath = filepath.Join(t.TempDir(), "pending.json")
	scanner.scanError = errors.New("offline")
	if _, err := RunVast(context.Background(), "secret-token", validRecipe(), options, deps); err == nil {
		t.Fatal("expected offline error")
	}
	cp, err := ReadCheckpoint(options.CheckpointPath)
	if err != nil || cp.InstanceID != 987 {
		t.Fatalf("checkpoint=%+v err=%v", cp, err)
	}
	if _, err := RunVast(context.Background(), "secret-token", validRecipe(), options, deps); err == nil {
		t.Fatal("pending deployment must block creation")
	}
	if api.createCalls != 1 || api.searchCalls != 1 {
		t.Fatal("duplicate paid operation")
	}
	options.Resume = true
	_, _ = RunVast(context.Background(), "secret-token", validRecipe(), options, deps)
	if api.createCalls != 1 || api.searchCalls != 1 {
		t.Fatal("resume searched or created")
	}
	info, _ := os.Stat(options.CheckpointPath)
	if info.Mode().Perm() != 0600 {
		t.Fatal("unsafe permissions")
	}
}

func TestCorruptCheckpointBlocksProvider(t *testing.T) {
	api, _, _, _, _, _, deps := successfulSetup()
	options := testOptions()
	options.CheckpointPath = filepath.Join(t.TempDir(), "pending.json")
	if err := os.WriteFile(options.CheckpointPath, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := RunVast(context.Background(), "token", validRecipe(), options, deps)
	if err == nil || api.createCalls != 0 || api.searchCalls != 0 {
		t.Fatal("corrupt journal allowed provider operations")
	}
}

func TestReadCheckpointRejectsUnsafeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending")
	cp := Checkpoint{Version: 1, InstanceID: 987, Recipe: validRecipe(), IdentityFile: "identity", KnownHostsDir: "hosts", Phase: "created"}
	if err := writeCheckpoint(path, cp); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadCheckpoint(path); err == nil {
		t.Fatal("unsafe permissions accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path, path+"-link"); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadCheckpoint(path + "-link"); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestTrustIgnoresScanOrderButNotChangedKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	raw := []byte("host ssh-ed25519 AAAA\nhost ssh-rsa BBBB\n")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	cp := Checkpoint{ApprovedHost: "host", ApprovedPort: 22, KnownHostsFile: path, HostKeysHash: hostKeysDigest(raw)}
	instance := vast.Instance{SSHHost: "host", SSHPort: 22}
	if !cp.trustMatches(instance, path, []byte("# comment\nhost ssh-rsa BBBB\nhost ssh-ed25519 AAAA\n")) {
		t.Fatal("unchanged keys required reconfirmation")
	}
	if cp.trustMatches(instance, path, []byte("host ssh-rsa EVIL\n")) {
		t.Fatal("changed key trusted")
	}
}
