package setup

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestProvisioningRetriesBrokenPipeBeforeTrustAndLaunch(t *testing.T) {
	api, _, _, trust, launcher, clock, deps := successfulSetup()
	scans := 0
	runner := &fakeCommandRunner{onOutput: func(cmd Command) ([]byte, error) {
		if cmd.Name == "ssh-keyscan" {
			scans++
			if scans < 3 {
				return nil, errors.New("write (ssh9.vast.ai): Broken pipe")
			}
			return []byte("gpu.example ssh-ed25519 AAAA\n"), nil
		}
		return []byte("256 SHA256:abc gpu.example (ED25519)\n"), nil
	}}
	deps.HostKeyScanner = SystemHostKeyScanner{Runner: runner}
	result, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
	if err != nil {
		t.Fatalf("temporary SSH failure aborted provisioning: %v", err)
	}
	if result.InstanceID != 987 || scans != 3 || api.createCalls != 1 || trust.calls != 1 || launcher.calls != 1 {
		t.Fatal("retry did not preserve single instance / trust / launch")
	}
	if len(clock.sleeps) < 3 {
		t.Fatal("SSH retries did not wait")
	}
}

func TestProvisioningSSHRetryStopsAndPreservesInstance(t *testing.T) {
	api, _, _, trust, launcher, clock, deps := successfulSetup()
	scans := 0
	deps.HostKeyScanner = SystemHostKeyScanner{Runner: &fakeCommandRunner{onOutput: func(Command) ([]byte, error) { scans++; return nil, errors.New("Broken pipe") }}}
	_, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
	if err == nil || !strings.Contains(err.Error(), "timed out waiting for SSH") {
		t.Fatalf("expected bounded SSH timeout: %v", err)
	}
	if scans < 2 || scans > 30 || clock.now.Sub(time.Unix(0, 0)) > 3*time.Minute {
		t.Fatal("unbounded retry")
	}
	if api.createCalls != 1 || trust.calls != 0 || launcher.calls != 0 {
		t.Fatal("failed scan mutated trust or launched server")
	}
}

func TestProvisioningSSHRetryHonorsCancellation(t *testing.T) {
	_, _, _, trust, launcher, _, deps := successfulSetup()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scans := 0
	deps.HostKeyScanner = SystemHostKeyScanner{Runner: &fakeCommandRunner{onOutput: func(Command) ([]byte, error) { scans++; cancel(); return nil, errors.New("Broken pipe") }}}
	_, err := RunVast(ctx, "token", validRecipe(), testOptions(), deps)
	if !errors.Is(err, context.Canceled) || scans != 1 || trust.calls != 0 || launcher.calls != 0 {
		t.Fatalf("cancellation ignored: %v", err)
	}
}

func TestProvisioningDoesNotRetryFingerprintFailure(t *testing.T) {
	_, _, _, trust, launcher, _, deps := successfulSetup()
	scans := 0
	deps.HostKeyScanner = SystemHostKeyScanner{Runner: &fakeCommandRunner{onOutput: func(cmd Command) ([]byte, error) {
		if cmd.Name == "ssh-keyscan" {
			scans++
			return []byte("invalid-key"), nil
		}
		return nil, errors.New("invalid fingerprint material")
	}}}
	_, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps)
	if err == nil || scans != 1 || trust.calls != 0 || launcher.calls != 0 {
		t.Fatal("invalid fingerprint was retried or trusted")
	}
}
