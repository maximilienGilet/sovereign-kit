package setup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/recipes"
)

type fakeCommandRunner struct {
	outputs  map[string][]byte
	errors   map[string]error
	calls    []Command
	onRun    func(Command) error
	onOutput func(Command) ([]byte, error)
}

func (f *fakeCommandRunner) Output(_ context.Context, command Command) ([]byte, error) {
	f.calls = append(f.calls, command)
	if f.onOutput != nil {
		return f.onOutput(command)
	}
	key := command.Name + " " + strings.Join(command.Args, " ")
	return f.outputs[key], f.errors[key]
}

func (f *fakeCommandRunner) Run(_ context.Context, command Command) error {
	f.calls = append(f.calls, command)
	if f.onRun != nil {
		return f.onRun(command)
	}
	key := command.Name + " " + strings.Join(command.Args, " ")
	return f.errors[key]
}

type fakeSSHKeyRegistry struct {
	has      bool
	checked  string
	added    string
	events   *[]string
	checkErr error
	addErr   error
}

func (registry *fakeSSHKeyRegistry) HasSSHKey(_ context.Context, publicKey string) (bool, error) {
	registry.checked = publicKey
	if registry.events != nil {
		*registry.events = append(*registry.events, "check-account")
	}
	return registry.has, registry.checkErr
}

func (registry *fakeSSHKeyRegistry) AddSSHKey(_ context.Context, publicKey string) error {
	registry.added = publicKey
	if registry.events != nil {
		*registry.events = append(*registry.events, "register-account")
	}
	return registry.addErr
}

type fakeIdentityOperator struct {
	confirmed bool
	calls     int
	path      string
	generate  bool
	events    *[]string
}

func (operator *fakeIdentityOperator) ConfirmIdentitySetup(_ context.Context, path string, generate bool) (bool, error) {
	operator.calls++
	operator.path = path
	operator.generate = generate
	if operator.events != nil {
		*operator.events = append(*operator.events, "confirm")
	}
	return operator.confirmed, nil
}

func TestPrepareVastIdentityGeneratesAndRegistersAfterConfirmation(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".ssh", "sovkit_vast_ed25519")
	events := []string{}
	runner := &fakeCommandRunner{
		outputs: map[string][]byte{
			"ssh-keygen -y -f " + path: []byte("ssh-ed25519 QUFBQQ== local\n"),
		},
		errors: map[string]error{},
		onRun: func(command Command) error {
			events = append(events, "generate")
			return os.WriteFile(path, []byte("private"), 0o600)
		},
	}
	registry := &fakeSSHKeyRegistry{events: &events}
	operator := &fakeIdentityOperator{confirmed: true, events: &events}

	if err := PrepareVastIdentity(context.Background(), path, registry, runner, operator); err != nil {
		t.Fatal(err)
	}
	if operator.calls != 1 || operator.path != path || !operator.generate {
		t.Fatalf("confirmation calls=%d path=%q generate=%v", operator.calls, operator.path, operator.generate)
	}
	if registry.checked != "ssh-ed25519 QUFBQQ== local" || registry.added != registry.checked {
		t.Fatalf("checked=%q added=%q", registry.checked, registry.added)
	}
	if want := []string{"confirm", "generate", "check-account", "register-account"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events=%v want=%v", events, want)
	}
}

func TestPrepareVastIdentityWithSSHKeygen(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".ssh", "sovkit_vast_ed25519")
	registry := &fakeSSHKeyRegistry{}
	operator := &fakeIdentityOperator{confirmed: true}

	if err := PrepareVastIdentity(context.Background(), path, registry, ExecRunner{}, operator); err != nil {
		t.Fatal(err)
	}
	if err := ValidateIdentityFile(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".pub"); err != nil {
		t.Fatalf("public key missing: %v", err)
	}
	if !strings.HasPrefix(registry.added, "ssh-ed25519 ") {
		t.Fatalf("registered key=%q", registry.added)
	}

	registry.has = true
	reuseOperator := &fakeIdentityOperator{}
	if err := PrepareVastIdentity(context.Background(), path, registry, ExecRunner{}, reuseOperator); err != nil {
		t.Fatal(err)
	}
	if reuseOperator.calls != 0 {
		t.Fatalf("existing registered identity asked for confirmation %d times", reuseOperator.calls)
	}
}

func TestPrepareVastIdentityDerivesMissingPublicKeyWithoutOverwritingPrivateKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity")
	if err := os.WriteFile(path, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeCommandRunner{
		outputs: map[string][]byte{
			"ssh-keygen -y -f " + path: []byte("ecdsa-sha2-nistp256 QUFBQQ== derived\n"),
		},
		errors: map[string]error{},
	}
	registry := &fakeSSHKeyRegistry{has: true}
	operator := &fakeIdentityOperator{confirmed: true}

	if err := PrepareVastIdentity(context.Background(), path, registry, runner, operator); err != nil {
		t.Fatal(err)
	}
	if operator.calls != 0 || registry.added != "" {
		t.Fatalf("confirmation calls=%d added=%q", operator.calls, registry.added)
	}
	if len(runner.calls) != 1 || runner.calls[0].Name != "ssh-keygen" || !reflect.DeepEqual(runner.calls[0].Args, []string{"-y", "-f", path}) {
		t.Fatalf("unexpected commands: %#v", runner.calls)
	}
}

func TestPrepareVastIdentityConfirmsBeforeRegisteringExistingKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity")
	if err := os.WriteFile(path, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	events := []string{}
	runner := &fakeCommandRunner{
		outputs: map[string][]byte{
			"ssh-keygen -y -f " + path: []byte("ssh-ed25519 QUFBQQ==\n"),
		},
		errors: map[string]error{},
	}
	registry := &fakeSSHKeyRegistry{events: &events}
	operator := &fakeIdentityOperator{confirmed: true, events: &events}

	if err := PrepareVastIdentity(context.Background(), path, registry, runner, operator); err != nil {
		t.Fatal(err)
	}
	if operator.calls != 1 || operator.generate || registry.added != "ssh-ed25519 QUFBQQ==" {
		t.Fatalf("confirmation calls=%d generate=%v added=%q", operator.calls, operator.generate, registry.added)
	}
	if want := []string{"check-account", "confirm", "register-account"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events=%v want=%v", events, want)
	}
}

func TestPrepareVastIdentityRefusesOrphanedPublicKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity")
	if err := os.WriteFile(path+".pub", []byte("ssh-ed25519 AAAA-orphan"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeCommandRunner{outputs: map[string][]byte{}, errors: map[string]error{}}
	registry := &fakeSSHKeyRegistry{}
	operator := &fakeIdentityOperator{confirmed: true}

	err := PrepareVastIdentity(context.Background(), path, registry, runner, operator)
	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("error=%v", err)
	}
	if operator.calls != 0 || len(runner.calls) != 0 || registry.checked != "" || registry.added != "" {
		t.Fatalf("operator=%d runner=%#v checked=%q added=%q", operator.calls, runner.calls, registry.checked, registry.added)
	}
}

func TestPrepareVastIdentityDeclineLeavesFilesystemAndAccountUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".ssh", "sovkit_vast_ed25519")
	runner := &fakeCommandRunner{outputs: map[string][]byte{}, errors: map[string]error{}}
	registry := &fakeSSHKeyRegistry{}
	operator := &fakeIdentityOperator{confirmed: false}

	err := PrepareVastIdentity(context.Background(), path, registry, runner, operator)
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("error=%v, want cancellation", err)
	}
	if len(runner.calls) != 0 || registry.checked != "" || registry.added != "" {
		t.Fatalf("runner=%#v checked=%q added=%q", runner.calls, registry.checked, registry.added)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("identity was created: %v", statErr)
	}
}
func TestExecRunnerFailuresIncludeBoundedSanitizedStderr(t *testing.T) {
	const keyMaterial = "gpu.example ssh-ed25519 AAAA-key-material"
	for _, runner := range []struct {
		name string
		run  func(context.Context, Command) ([]byte, error)
	}{
		{name: "output", run: (ExecRunner{}).Output},
		{name: "run", run: func(ctx context.Context, command Command) ([]byte, error) {
			return nil, (ExecRunner{}).Run(ctx, command)
		}},
	} {
		t.Run(runner.name, func(t *testing.T) {
			output, err := runner.run(context.Background(), Command{
				Name:  "sh",
				Args:  []string{"-c", "cat >&2; printf ' visible \\n' >&2; exit 1"},
				Stdin: []byte(keyMaterial),
			})
			if output != nil {
				t.Fatalf("output = %q, want nil", output)
			}
			if err == nil || !strings.Contains(err.Error(), "sh failed") || !strings.Contains(err.Error(), "visible") {
				t.Fatalf("error = %v, want command and stderr", err)
			}
			if strings.Contains(err.Error(), keyMaterial) {
				t.Fatalf("error exposed stdin key material: %v", err)
			}
		})
	}
}

func TestExecRunnerFailuresTruncateStderr(t *testing.T) {
	stderr := strings.Repeat("x", 2048)
	for _, runner := range []struct {
		name string
		run  func(context.Context, Command) ([]byte, error)
	}{
		{name: "output", run: (ExecRunner{}).Output},
		{name: "run", run: func(ctx context.Context, command Command) ([]byte, error) {
			return nil, (ExecRunner{}).Run(ctx, command)
		}},
	} {
		t.Run(runner.name, func(t *testing.T) {
			output, err := runner.run(context.Background(), Command{
				Name: "sh",
				Args: []string{"-c", "printf '%*s' 3000 '' | tr ' ' x >&2; exit 1"},
			})
			if output != nil {
				t.Fatalf("output = %q, want nil", output)
			}
			if err == nil || !strings.Contains(err.Error(), stderr) {
				t.Fatalf("error = %v, want bounded stderr", err)
			}
			if strings.Contains(err.Error(), stderr+"x") {
				t.Fatalf("error contains more than 2 KiB stderr: %d", len(err.Error()))
			}
		})
	}
}

func TestSystemHostKeyScannerScansAndFingerprints(t *testing.T) {
	runner := &fakeCommandRunner{
		outputs: map[string][]byte{
			"ssh-keyscan -T 10 -p 22022 gpu.example": []byte("gpu.example ssh-ed25519 AAAA\n"),
			"ssh-keygen -lf - -E sha256":             []byte("256 SHA256:first gpu.example (ED25519)\n256 SHA256:second gpu.example (ED25519)\n"),
		},
		errors: map[string]error{},
	}

	got, err := (SystemHostKeyScanner{Runner: runner}).Scan(context.Background(), "gpu.example", 22022)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, HostKeys{
		Raw:          []byte("gpu.example ssh-ed25519 AAAA\n"),
		Fingerprints: []string{"256 SHA256:first gpu.example (ED25519)", "256 SHA256:second gpu.example (ED25519)"},
	}) {
		t.Fatalf("unexpected host keys: %#v", got)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("expected two commands, got %#v", runner.calls)
	}
	if !reflect.DeepEqual(runner.calls[0], Command{Name: "ssh-keyscan", Args: []string{"-T", "10", "-p", "22022", "gpu.example"}}) {
		t.Fatalf("unexpected scan command: %#v", runner.calls[0])
	}
	if !reflect.DeepEqual(runner.calls[1], Command{Name: "ssh-keygen", Args: []string{"-lf", "-", "-E", "sha256"}, Stdin: []byte("gpu.example ssh-ed25519 AAAA\n")}) {
		t.Fatalf("unexpected fingerprint command: %#v", runner.calls[1])
	}
}

func TestSystemHostKeyScannerRejectsBlankScan(t *testing.T) {
	runner := &fakeCommandRunner{outputs: map[string][]byte{"ssh-keyscan -T 10 -p 22022 gpu.example": []byte(" \n")}, errors: map[string]error{}}
	_, err := (SystemHostKeyScanner{Runner: runner}).Scan(context.Background(), "gpu.example", 22022)
	if err == nil || !strings.Contains(err.Error(), "scan") {
		t.Fatalf("expected blank scan error, got %v", err)
	}
}

func TestSystemHostKeyScannerRejectsBlankFingerprint(t *testing.T) {
	runner := &fakeCommandRunner{outputs: map[string][]byte{
		"ssh-keyscan -T 10 -p 22022 gpu.example": []byte("gpu.example ssh-ed25519 AAAA\n"),
		"ssh-keygen -lf - -E sha256":             []byte(" \n"),
	}, errors: map[string]error{}}
	_, err := (SystemHostKeyScanner{Runner: runner}).Scan(context.Background(), "gpu.example", 22022)
	if err == nil || !strings.Contains(err.Error(), "fingerprint") {
		t.Fatalf("expected blank fingerprint error, got %v", err)
	}
}

func TestValidateIdentityFileAcceptsReadableRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(path, []byte("private key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateIdentityFile(path); err != nil {
		t.Fatal(err)
	}
}

func TestValidateIdentityFileRejectsMissingPath(t *testing.T) {
	err := ValidateIdentityFile(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("expected missing identity error")
	}
}

func TestFileTrustStoreRaceRejectsChangedWinner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	original := []byte("host ssh-ed25519 AAAA\n")
	winner := []byte("host ssh-ed25519 BBBB\n")
	previousLink := linkTrustStoreFile
	t.Cleanup(func() { linkTrustStoreFile = previousLink })
	linkTrustStoreFile = func(oldname, newname string) error {
		if err := os.WriteFile(newname, winner, 0o600); err != nil {
			return err
		}
		return os.Link(oldname, newname)
	}

	err := (FileTrustStore{}).Save(path, original)
	if err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("expected changed-key refusal, got %v", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !reflect.DeepEqual(got, winner) {
		t.Fatalf("winner was replaced: %q", got)
	}
}

func TestFileTrustStoreRaceAllowsIdenticalWinner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	contents := []byte("host ssh-ed25519 AAAA\n")
	previousLink := linkTrustStoreFile
	t.Cleanup(func() { linkTrustStoreFile = previousLink })
	linkTrustStoreFile = func(oldname, newname string) error {
		if err := os.WriteFile(newname, contents, 0o644); err != nil {
			return err
		}
		return os.Link(oldname, newname)
	}

	if err := (FileTrustStore{}).Save(path, contents); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)

	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("winner mode = %o", info.Mode().Perm())
	}
}
func TestControlledSGLangCommandOnlyTrustsRemoteCodeWhenRecipeAllows(t *testing.T) {
	untrusted := validRecipe()
	untrusted.Model.TrustRemoteCode = false
	command, err := ControlledSGLangCommand(untrusted)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(command, "--trust-remote-code") {
		t.Fatalf("untrusted custom model command=%s", command)
	}

	trusted := validRecipe()
	trusted.Model.TrustRemoteCode = true
	command, err = ControlledSGLangCommand(trusted)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(command, "--trust-remote-code") {
		t.Fatalf("reviewed recipe command=%s", command)
	}
}

func TestValidateIdentityFileRejectsDirectory(t *testing.T) {
	if err := ValidateIdentityFile(t.TempDir()); err == nil {
		t.Fatal("expected directory identity error")
	}
}

func TestFileTrustStoreCreatesPrivateAtomicFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "known_hosts")
	contents := []byte("host ssh-ed25519 AAAA\n")
	if err := (FileTrustStore{}).Save(path, contents); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, contents) {
		t.Fatalf("unexpected contents: %q", got)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("known hosts mode = %o", info.Mode().Perm())
	}
	if info, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o700 {
		t.Fatalf("known hosts directory mode = %o", info.Mode().Perm())
	}
}

func TestFileTrustStoreAllowsIdenticalBytesAndRejectsChangedBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	original := []byte("host ssh-ed25519 AAAA\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (FileTrustStore{}).Save(path, original); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (FileTrustStore{}).Save(path, []byte("host ssh-ed25519 BBBB\n")); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("expected changed-key refusal, got %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, original) {
		t.Fatalf("changed bytes overwrote original: %q", got)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o644 {
		t.Fatalf("refused change altered mode = %o", info.Mode().Perm())
	}
}

func TestRealClockSleepHonorsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (RealClock{}).Sleep(ctx, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestStrictSSHLauncherUsesReviewedCommand(t *testing.T) {
	r, err := recipes.QwenStudio()
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeCommandRunner{errors: map[string]error{}}
	ssh := config.SSH{Host: "gpu.example", Port: 22022, User: "root", IdentityFile: "/tmp/vast-987_id", KnownHostsFile: "/tmp/vast-987_known_hosts"}
	if err := (StrictSSHLauncher{Runner: runner}).Launch(context.Background(), ssh, r); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one launch command, got %#v", runner.calls)
	}
	command := runner.calls[0]
	joined := strings.Join(command.Args, " ")
	for _, expected := range []string{
		"-o", "BatchMode=yes", "IdentitiesOnly=yes", "StrictHostKeyChecking=yes", "UserKnownHostsFile=/tmp/vast-987_known_hosts",
		"python3 -m 'sglang.launch_server'", "'--model-path' 'RadixArk/Qwen3.8-27B-NVFP4'", "'--revision' '319f741cce68d7914884900c138a1fbb70a42f30'", "'--context-length' '262144'", "'--max-running-requests' '5'", "'--host' '127.0.0.1'", "'--port' '30000'",
		"nohup", ">/workspace/sovkit-sglang.log", "2>&1", "</dev/null", "&",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("command missing %q: %s", expected, joined)
		}
	}
	if strings.Contains(joined, r.Runtime.Image) || !strings.Contains(joined, "-p 22022 -- root@gpu.example") {
		t.Fatalf("unexpected launch command: %s", joined)
	}
}

func TestStrictSSHLauncherQuotesMaliciousModelRepository(t *testing.T) {
	r, err := recipe.CustomHuggingFace("owner/model'; touch /tmp/pwned; echo '", "319f741cce68d7914884900c138a1fbb70a42f30", true)
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeCommandRunner{errors: map[string]error{}}
	ssh := config.SSH{Host: "gpu.example", Port: 22022, User: "root", IdentityFile: "/tmp/id", KnownHostsFile: "/tmp/known_hosts"}
	if err := (StrictSSHLauncher{Runner: runner}).Launch(context.Background(), ssh, r); err != nil {
		t.Fatal(err)
	}
	remote := runner.calls[0].Args[len(runner.calls[0].Args)-1]
	if !strings.Contains(remote, "'--model-path' "+shellQuote(r.Model.Repository)) {
		t.Fatalf("malicious model was not safely quoted: %s", remote)
	}
}
func TestStrictSSHLauncherQuotesOptionLikeModelRepository(t *testing.T) {
	r, err := recipe.CustomHuggingFace("--a/b;echo pwned;#", "319f741cce68d7914884900c138a1fbb70a42f30", true)
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeCommandRunner{errors: map[string]error{}}
	ssh := config.SSH{Host: "gpu.example", Port: 22022, User: "root", IdentityFile: "/tmp/id", KnownHostsFile: "/tmp/known_hosts"}
	if err := (StrictSSHLauncher{Runner: runner}).Launch(context.Background(), ssh, r); err != nil {
		t.Fatal(err)
	}
	remote := runner.calls[0].Args[len(runner.calls[0].Args)-1]
	if !strings.Contains(remote, "'--model-path' '--a/b;echo pwned;#'") {
		t.Fatalf("option-like model was not safely quoted: %s", remote)
	}
	if strings.Contains(remote, "--model-path --a/b;echo pwned;#") {
		t.Fatalf("raw option-like model segment was injected: %s", remote)
	}
}

func TestStrictSSHLauncherRejectsNonSGLangRecipe(t *testing.T) {
	r, err := recipe.CustomHuggingFace("owner/model", "319f741cce68d7914884900c138a1fbb70a42f30", true)
	if err != nil {
		t.Fatal(err)
	}
	r.Runtime.Engine = "llama-cpp"
	runner := &fakeCommandRunner{errors: map[string]error{}}
	ssh := config.SSH{Host: "gpu.example", Port: 22022, User: "root", IdentityFile: "/tmp/id", KnownHostsFile: "/tmp/known_hosts"}
	if err := (StrictSSHLauncher{Runner: runner}).Launch(context.Background(), ssh, r); err == nil {
		t.Fatal("expected non-SGLang recipe rejection")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("rejected recipe invoked runner: %#v", runner.calls)
	}
}

func TestStrictSSHLauncherUsesReviewedVLLMCommand(t *testing.T) {
	r := recipe.Recipe{
		Version: 1, ID: "qwen-solo-lab", Name: "Qwen Solo Lab", Kind: "speculative-text-generation",
		Profile: recipe.Profile{Status: "experimental", Summary: "Qwen3.8 on one RTX 5090", Evidence: "Requires live gauntlet"},
		Runtime: recipe.Runtime{
			Engine: "vllm", Image: "ghcr.io/example/vllm@sha256:e21dd539b36ea7842101393ec3fe3b0d453626cd8251e4d4db33af0cd97d7f0b",
			Quantization: "modelopt", KVCacheDType: "turboquant_4bit_nc", GPUMemoryUtilization: 0.94, DisableAsyncScheduling: true,
		},
		Model:       recipe.Model{Repository: "gittensor-model-hub/Qwen3.8-27B-NVFP4-RTX5090-LMHead4", Revision: "f7866ff274f7b2ab34d9d828a7bccff51919b711", TrustRemoteCode: true},
		Speculative: &recipe.Speculative{Algorithm: "qwen3_5_mtp", NumDraftTokens: 4},
		Serve:       recipe.Serve{ContextWindow: 200000, MaxOutputTokens: 16384, MaxRunningRequests: 1},
		Requirements: recipe.Requirements{
			GPUModel: "RTX 5090", GPUCount: 1, StrictGPU: true, MinimumVRAMGB: 32, MinimumDiskGB: 100,
		},
	}
	runner := &fakeCommandRunner{errors: map[string]error{}}
	ssh := config.SSH{Host: "gpu.example", Port: 22022, User: "root", IdentityFile: "/tmp/id", KnownHostsFile: "/tmp/known_hosts"}
	if err := (StrictSSHLauncher{Runner: runner}).Launch(context.Background(), ssh, r); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("expected launch and readiness commands, got %#v", runner.calls)
	}
	remote := runner.calls[0].Args[len(runner.calls[0].Args)-1]
	for _, expected := range []string{
		"vllm serve", "'gittensor-model-hub/Qwen3.8-27B-NVFP4-RTX5090-LMHead4'",
		"'--revision' 'f7866ff274f7b2ab34d9d828a7bccff51919b711'",
		"'--quantization' 'modelopt'", "'--kv-cache-dtype' 'turboquant_4bit_nc'",
		"'--gpu-memory-utilization' '0.94'", "'--max-model-len' '200000'", "'--max-num-seqs' '1'",
		"'--no-async-scheduling'", `'{"method":"qwen3_5_mtp","num_speculative_tokens":4}'`,
		"'--reasoning-parser' 'qwen3'", "'--enable-auto-tool-choice'", "'--tool-call-parser' 'qwen3_xml'",
		"'--host' '127.0.0.1'", "'--port' '30000'", ">/workspace/sovkit-vllm.log", ">/workspace/sovkit-vllm.pid",
	} {
		if !strings.Contains(remote, expected) {
			t.Fatalf("vLLM command missing %q: %s", expected, remote)
		}
	}
	if strings.Contains(remote, r.Runtime.Image) {
		t.Fatalf("runtime image leaked into remote command: %s", remote)
	}
	readiness := runner.calls[1].Args[len(runner.calls[1].Args)-1]
	for _, expected := range []string{"http://127.0.0.1:30000/health", "/workspace/sovkit-vllm.pid", "tail -n 80 /workspace/sovkit-vllm.log", ">&2"} {
		if !strings.Contains(readiness, expected) {
			t.Fatalf("readiness command missing %q: %s", expected, readiness)
		}
	}
}

func TestStrictSSHLauncherSurfacesVLLMStartupLog(t *testing.T) {
	r := recipe.Recipe{
		Version: 1, ID: "qwen-solo-lab", Name: "Qwen Solo Lab", Kind: "speculative-text-generation",
		Profile: recipe.Profile{Status: "experimental"},
		Runtime: recipe.Runtime{
			Engine: "vllm", Image: "ghcr.io/example/vllm@sha256:e21dd539b36ea7842101393ec3fe3b0d453626cd8251e4d4db33af0cd97d7f0b",
			Quantization: "modelopt", KVCacheDType: "turboquant_4bit_nc", GPUMemoryUtilization: 0.94, DisableAsyncScheduling: true,
		},
		Model:       recipe.Model{Repository: "gittensor-model-hub/Qwen3.8-27B-NVFP4-RTX5090-LMHead4", Revision: "f7866ff274f7b2ab34d9d828a7bccff51919b711", TrustRemoteCode: true},
		Speculative: &recipe.Speculative{Algorithm: "qwen3_5_mtp", NumDraftTokens: 4},
		Serve:       recipe.Serve{ContextWindow: 200000, MaxOutputTokens: 16384, MaxRunningRequests: 1},
	}
	_, readinessErr := (ExecRunner{}).Output(context.Background(), Command{
		Name: "sh",
		Args: []string{"-c", "printf 'CUDA out of memory while loading model' >&2; exit 1"},
	})
	if readinessErr == nil {
		t.Fatal("expected production command runner failure")
	}
	runner := &fakeCommandRunner{
		errors: map[string]error{},
		onOutput: func(Command) ([]byte, error) {
			return nil, readinessErr
		},
	}
	ssh := config.SSH{Host: "gpu.example", Port: 22022, User: "root", IdentityFile: "/tmp/id", KnownHostsFile: "/tmp/known_hosts"}
	err := (StrictSSHLauncher{Runner: runner}).Launch(context.Background(), ssh, r)
	if err == nil || !strings.Contains(err.Error(), "vLLM readiness") || !strings.Contains(err.Error(), "CUDA out of memory") {
		t.Fatalf("error = %v, want readiness error with startup log", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("expected launch and readiness commands, got %#v", runner.calls)
	}
}

func TestDualMaxOOMRecommendsSaferCapacityProfiles(t *testing.T) {
	r, err := recipes.QwenSoloDualMax()
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []struct {
		name string
		run  func(StrictSSHLauncher, config.SSH, recipe.Recipe) error
	}{
		{"launch", func(launcher StrictSSHLauncher, ssh config.SSH, r recipe.Recipe) error {
			return launcher.Launch(context.Background(), ssh, r)
		}},
		{"reconcile", func(launcher StrictSSHLauncher, ssh config.SSH, r recipe.Recipe) error {
			return launcher.Reconcile(context.Background(), ssh, r)
		}},
	} {
		t.Run(operation.name, func(t *testing.T) {
			runner := &fakeCommandRunner{onOutput: func(Command) ([]byte, error) {
				return []byte("dead\ncudaMalloc failed: out of memory\n"), nil
			}}
			ssh := config.SSH{Host: "gpu.example", Port: 22022, User: "root", IdentityFile: "/tmp/id", KnownHostsFile: "/tmp/known_hosts"}
			err := operation.run(StrictSSHLauncher{Runner: runner}, ssh, r)
			if err == nil || !strings.Contains(err.Error(), "out of memory") || !strings.Contains(err.Error(), "Full Context") || !strings.Contains(err.Error(), "Dual") {
				t.Fatalf("error = %v, want retained OOM cause and safer profile guidance", err)
			}
		})
	}
}

func TestControlledVLLMCommandQuotesMaliciousModelRepository(t *testing.T) {
	r := recipe.Recipe{
		Version: 1, ID: "qwen-solo-lab", Name: "Qwen Solo Lab", Kind: "speculative-text-generation",
		Profile: recipe.Profile{Status: "experimental", Summary: "Qwen3.8 on one RTX 5090", Evidence: "Requires live gauntlet"},
		Runtime: recipe.Runtime{
			Engine: "vllm", Image: "ghcr.io/example/vllm@sha256:e21dd539b36ea7842101393ec3fe3b0d453626cd8251e4d4db33af0cd97d7f0b",
			Quantization: "modelopt", KVCacheDType: "turboquant_4bit_nc", GPUMemoryUtilization: 0.94, DisableAsyncScheduling: true,
		},
		Model:       recipe.Model{Repository: "owner/model'; touch /tmp/pwned; echo '", Revision: "f7866ff274f7b2ab34d9d828a7bccff51919b711", TrustRemoteCode: true},
		Speculative: &recipe.Speculative{Algorithm: "qwen3_5_mtp", NumDraftTokens: 4},
		Serve:       recipe.Serve{ContextWindow: 200000, MaxOutputTokens: 16384, MaxRunningRequests: 1},
	}
	remote, err := ControlledVLLMCommand(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(remote, "vllm serve "+shellQuote(r.Model.Repository)) || strings.Contains(remote, "serve owner/model'; touch") {
		t.Fatalf("model repository was not safely quoted: %s", remote)
	}
}
