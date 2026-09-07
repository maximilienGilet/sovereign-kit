package route

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
)

func forwardFixture(t *testing.T) (TunnelSpec, string) {
	t.Helper()
	dir := t.TempDir()
	identity := filepath.Join(dir, "identity")
	knownHosts := filepath.Join(dir, "known_hosts")
	if err := os.WriteFile(identity, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(knownHosts, []byte("host ssh-ed25519 AAAA"), 0o600); err != nil {
		t.Fatal(err)
	}
	return TunnelSpec{
		SSHHost: "ssh.example.test", SSHPort: 22022, SSHUser: "root",
		IdentityFile: identity, KnownHostsFile: knownHosts,
		LocalHost: "127.0.0.1", LocalPort: 30001,
		RemoteHost: "127.0.0.1", RemotePort: 30000,
	}, dir
}

func argv(cmd []string) string { return strings.Join(cmd, " ") }

func TestForwardCommandStrictEnforcesPinnedKey(t *testing.T) {
	spec, _ := forwardFixture(t)
	command, err := ForwardCommand(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	text := argv(command.Args)
	for _, want := range []string{
		"StrictHostKeyChecking=yes",
		"-L", "127.0.0.1:30001:127.0.0.1:30000",
		"-i", spec.IdentityFile,
		"-p", "22022",
		"root@ssh.example.test",
		"BatchMode=yes",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
	if strings.Contains(text, "accept-new") {
		t.Fatalf("strict forward must not accept new keys: %q", text)
	}
}

func TestForwardCommandAcceptNewPinsFirstContact(t *testing.T) {
	spec, _ := forwardFixture(t)
	spec.AcceptNewHostKey = true
	command, err := ForwardCommand(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if text := argv(command.Args); !strings.Contains(text, "StrictHostKeyChecking=accept-new") {
		t.Fatalf("missing accept-new in %q", text)
	}
}

func TestForwardCommandRejectsBadSpec(t *testing.T) {
	spec, _ := forwardFixture(t)
	bad := spec
	bad.IdentityFile = filepath.Join(t.TempDir(), "missing")
	if _, err := ForwardCommand(context.Background(), bad); err == nil {
		t.Fatal("expected missing identity error")
	}
	bad = spec
	bad.LocalHost = "0.0.0.0"
	if _, err := ForwardCommand(context.Background(), bad); err == nil {
		t.Fatal("expected non-loopback refusal")
	}
	bad = spec
	bad.LocalPort = 0
	if _, err := ForwardCommand(context.Background(), bad); err == nil {
		t.Fatal("expected bad port refusal")
	}
}

func configForSpec(spec TunnelSpec) config.Config {
	return config.Config{
		Route: config.Route{
			LocalHost: spec.LocalHost, LocalPort: spec.LocalPort,
			RemoteHost: spec.RemoteHost, RemotePort: spec.RemotePort,
		},
		SSH: config.SSH{
			Host: spec.SSHHost, Port: spec.SSHPort, User: spec.SSHUser,
			IdentityFile: spec.IdentityFile, KnownHostsFile: spec.KnownHostsFile,
		},
	}
}

func TestCommandContextKeepsLegacyArgv(t *testing.T) {
	spec, _ := forwardFixture(t)
	spec.LocalPort, spec.RemotePort = 30000, 30000
	command, err := CommandContext(context.Background(), configForSpec(spec))
	if err != nil {
		t.Fatal(err)
	}
	text := argv(command.Args)
	for _, want := range []string{
		"StrictHostKeyChecking=yes",
		"127.0.0.1:30000:127.0.0.1:30000",
		"root@ssh.example.test",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
}
