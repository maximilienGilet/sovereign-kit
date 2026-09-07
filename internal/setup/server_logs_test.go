package setup

import (
	"context"
	"errors"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"os/exec"
	"strings"
	"testing"
)

func TestServerLogsArriveBeforeReadinessAndAreRedacted(t *testing.T) {
	calls := 0
	var snapshots []ServerLogs
	runner := &fakeCommandRunner{onOutput: func(Command) ([]byte, error) {
		calls++
		if calls == 1 {
			return []byte("waiting\nDownloading token-secret\nAuthorization: Bearer sensitive\n"), nil
		}
		return []byte("ready\nModel loaded\n"), nil
	}}
	launcher := StrictSSHLauncher{Runner: runner, Clock: &fakeClock{}, LogSecrets: []string{"token-secret"}, OnLogs: func(logs ServerLogs) { snapshots = append(snapshots, logs) }}
	err := launcher.waitServerReady(context.Background(), config.SSH{Host: "host", Port: 22, User: "root", IdentityFile: "/key", KnownHostsFile: "/hosts"}, "sglang")
	if err != nil || calls != 2 || len(snapshots) != 2 {
		t.Fatalf("logs/readiness flow failed: %v", err)
	}
	if strings.Contains(snapshots[0].Text, "token-secret") || strings.Contains(snapshots[0].Text, "sensitive") {
		t.Fatal("secret exposed")
	}
	if !strings.Contains(snapshots[0].Text, "Downloading") || !strings.Contains(snapshots[1].Text, "Model loaded") {
		t.Fatal("live logs missing")
	}
}

func TestServerLogPollingStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	launcher := StrictSSHLauncher{Runner: &fakeCommandRunner{onOutput: func(Command) ([]byte, error) { calls++; return []byte("waiting\nLoading\n"), nil }}, OnLogs: func(ServerLogs) { cancel() }}
	err := launcher.waitServerReady(ctx, config.SSH{}, "sglang")
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("poller did not stop: %v (%d reads)", err, calls)
	}
}

func TestServerReadinessCommandProducesParseableStatus(t *testing.T) {
	launcher := StrictSSHLauncher{Runner: &fakeCommandRunner{onOutput: func(cmd Command) ([]byte, error) {
		// Execute the real shell protocol locally; replace only remote I/O.
		return exec.Command("sh", "-c", "curl() { return 0; }; tail() { printf 'model loaded\\n'; }; "+cmd.Args[len(cmd.Args)-1]).CombinedOutput()
	}}}
	if err := launcher.waitServerReady(context.Background(), config.SSH{}, "sglang"); err != nil {
		t.Fatal(err)
	}
}

func TestDeadServerKeepsFinalLogs(t *testing.T) {
	var latest ServerLogs
	launcher := StrictSSHLauncher{Runner: &fakeCommandRunner{onOutput: func(Command) ([]byte, error) { return []byte("dead\nCUDA out of memory\n"), nil }}, OnLogs: func(s ServerLogs) { latest = s }}
	err := launcher.waitServerReady(context.Background(), config.SSH{}, "vllm")
	if err == nil || !strings.Contains(latest.Text, "CUDA out of memory") {
		t.Fatal("dead server lost logs or reported ready")
	}
}

func TestReconcileRefusalStillPublishesRedactedLogs(t *testing.T) {
	refusal := errors.New("Previous launch outcome is uncertain")
	var latest ServerLogs
	reads := 0
	runner := &fakeCommandRunner{onRun: func(Command) error { return refusal }, onOutput: func(cmd Command) ([]byte, error) {
		reads++
		remote := cmd.Args[len(cmd.Args)-1]
		if !strings.HasPrefix(remote, "tail -c 16384 ") {
			t.Fatal("diagnostic attempted more than a bounded log read")
		}
		return []byte("ModuleNotFoundError: sglang\nsecret-value"), nil
	}}
	launcher := StrictSSHLauncher{Runner: runner, LogSecrets: []string{"secret-value"}, OnLogs: func(s ServerLogs) { latest = s }}
	err := launcher.Reconcile(context.Background(), config.SSH{Host: "host", Port: 22, User: "root", IdentityFile: "/key", KnownHostsFile: "/hosts"}, validRecipe())
	if !errors.Is(err, refusal) || reads != 1 || len(runner.calls) != 2 {
		t.Fatalf("refusal/diagnostic flow: %v reads=%d", err, reads)
	}
	if !strings.Contains(latest.Text, "ModuleNotFoundError") || strings.Contains(latest.Text, "secret-value") {
		t.Fatal("failure logs missing or unredacted")
	}
}
