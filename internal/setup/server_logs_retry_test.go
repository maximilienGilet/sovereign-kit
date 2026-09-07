package setup

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
)

func TestReadinessRetriesTransientReadAndKeepsLogs(t *testing.T) {
	calls := 0
	var snapshots []ServerLogs
	runner := &fakeCommandRunner{onOutput: func(Command) ([]byte, error) {
		calls++
		switch calls {
		case 1:
			return []byte("waiting\nDownloading model\n"), nil
		case 2:
			return nil, context.DeadlineExceeded
		default:
			return []byte("ready\nLoaded\n"), nil
		}
	}}
	l := StrictSSHLauncher{Runner: runner, Clock: &fakeClock{}, OnLogs: func(s ServerLogs) { snapshots = append(snapshots, s) }}
	if err := l.waitServerReady(context.Background(), config.SSH{}, "llama"); err != nil {
		t.Fatal(err)
	}
	if calls != 3 || len(snapshots) != 3 {
		t.Fatalf("reads=%d snapshots=%d", calls, len(snapshots))
	}
	if !strings.Contains(snapshots[1].Text, "Downloading model") || !strings.Contains(snapshots[1].Text, "retry") {
		t.Fatal("lost logs or retry feedback")
	}
}

func TestReadinessDoesNotRetrySecurityOrUnknownErrors(t *testing.T) {
	for _, message := range []string{"Host key verification failed", "Permission denied (publickey)", "REMOTE HOST IDENTIFICATION HAS CHANGED! Connection closed", "unexpected failure"} {
		calls := 0
		l := StrictSSHLauncher{Runner: &fakeCommandRunner{onOutput: func(Command) ([]byte, error) { calls++; return nil, errors.New(message) }}, Clock: &fakeClock{}}
		if err := l.waitServerReady(context.Background(), config.SSH{}, "llama"); err == nil || calls != 1 {
			t.Fatalf("%s: calls=%d err=%v", message, calls, err)
		}
	}
}

func TestReadinessTransientRetryBudgetIsBounded(t *testing.T) {
	calls := 0
	l := StrictSSHLauncher{Runner: &fakeCommandRunner{onOutput: func(Command) ([]byte, error) { calls++; return nil, errors.New("ssh: Connection reset by peer") }}, Clock: &fakeClock{}}
	if err := l.waitServerReady(context.Background(), config.SSH{}, "llama"); err == nil || calls != 6 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestReadinessKilledCommandNeedsDeadlineEvidence(t *testing.T) {
	err := errors.New("ssh failed: signal: killed")
	if !transientReadinessRead(err, context.DeadlineExceeded) {
		t.Fatal("lost timeout behind killed process")
	}
	if transientReadinessRead(err, nil) {
		t.Fatal("unexplained process kill treated as network timeout")
	}
}

func TestReadinessCancellationDuringRetryDoesNotReadAgain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	l := StrictSSHLauncher{Runner: &fakeCommandRunner{onOutput: func(Command) ([]byte, error) { calls++; return nil, context.DeadlineExceeded }}, OnLogs: func(ServerLogs) { cancel() }}
	if err := l.waitServerReady(ctx, config.SSH{}, "llama"); !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}
