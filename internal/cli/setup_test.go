package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
)

func TestSetupWritesTheBuiltInStudioProfileToToml(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	identity := filepath.Join(dir, "identity")
	knownHosts := filepath.Join(dir, "known_hosts")
	if err := os.WriteFile(identity, []byte("private key placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(knownHosts, []byte("gpu.example.test ssh-ed25519 AAAA"), 0o600); err != nil {
		t.Fatal(err)
	}
	input := bytes.NewBufferString("gpu.example.test\n\nsovkit\n" + identity + "\n" + knownHosts + "\n")
	output := &bytes.Buffer{}

	if err := Setup(input, output, configPath, "alice"); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SSH.Host != "gpu.example.test" || cfg.SSH.Port != 22 || cfg.SSH.User != "sovkit" {
		t.Fatalf("unexpected setup config: %#v", cfg)
	}
	if !bytes.Contains(output.Bytes(), []byte("qwen-studio")) || !bytes.Contains(output.Bytes(), []byte("Configuration saved")) {
		t.Fatalf("unexpected setup output: %s", output.String())
	}
}

func TestSetupRefusesUnreadableSSHFiles(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	input := bytes.NewBufferString("gpu.example.test\n22\nsovkit\n/missing/key\n/missing/known_hosts\n")

	err := Setup(input, &bytes.Buffer{}, configPath, "alice")
	if err == nil {
		t.Fatal("expected setup to reject unreadable SSH files")
	}
}
