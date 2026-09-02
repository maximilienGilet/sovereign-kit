package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
)

func TestRunWithSetupWritesPrivateRouteConfig(t *testing.T) {
	dir := t.TempDir()
	identity := filepath.Join(dir, "identity")
	knownHosts := filepath.Join(dir, "known_hosts")
	for _, path := range []string{identity, knownHosts} {
		if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	configPath := filepath.Join(dir, "config.toml")
	input := strings.NewReader("gpu.example\n\nubuntu\n" + identity + "\n" + knownHosts + "\n")
	var output bytes.Buffer
	if err := runWith([]string{"setup"}, input, &output, configPath, "ubuntu"); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SSH.Host != "gpu.example" || cfg.SSH.User != "ubuntu" {
		t.Fatalf("unexpected config: %#v", cfg.SSH)
	}
}
