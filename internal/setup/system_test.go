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
	outputs map[string][]byte
	errors  map[string]error
	calls   []Command
}

func (f *fakeCommandRunner) Output(_ context.Context, command Command) ([]byte, error) {
	f.calls = append(f.calls, command)
	key := command.Name + " " + strings.Join(command.Args, " ")
	return f.outputs[key], f.errors[key]
}

func (f *fakeCommandRunner) Run(_ context.Context, command Command) error {
	f.calls = append(f.calls, command)
	key := command.Name + " " + strings.Join(command.Args, " ")
	return f.errors[key]
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
		"sglang serve", "'--model-path' 'RadixArk/Qwen3.8-27B-NVFP4'", "'--revision' '319f741cce68d7914884900c138a1fbb70a42f30'", "'--context-length' '262144'", "'--max-running-requests' '5'", "'--host' '127.0.0.1'", "'--port' '30000'",
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
