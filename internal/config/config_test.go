package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAcceptsTheBuiltInStudioRoute(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `version = 1
profile = "qwen-studio"

[provider]
kind = "vast"
instance_id = 12345678

[route]
local_host = "127.0.0.1"
local_port = 30000
remote_host = "127.0.0.1"
remote_port = 30000

[ssh]
host = "gpu.example.test"
port = 22
user = "sovkit"
identity_file = "/home/alice/.ssh/sovkit"
known_hosts_file = "/home/alice/.ssh/sovkit_known_hosts"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Profile != "qwen-studio" || cfg.Provider.Kind != "vast" || cfg.Provider.InstanceID != 12345678 || cfg.Route.LocalHost != "127.0.0.1" || cfg.SSH.Port != 22 {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestLoadRejectsAPublicInferenceBinding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `version = 1
profile = "qwen-studio"

[provider]
kind = "vast"
instance_id = 12345678

[route]
local_host = "0.0.0.0"
local_port = 30000
remote_host = "127.0.0.1"
remote_port = 30000

[ssh]
host = "gpu.example.test"
port = 22
user = "sovkit"
identity_file = "/home/alice/.ssh/sovkit"
known_hosts_file = "/home/alice/.ssh/sovkit_known_hosts"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("expected loopback validation error, got %v", err)
	}
}

func TestSaveWritesOwnerOnlyToml(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.toml")
	cfg := Studio("gpu.example.test", 22, "sovkit", "/home/alice/.ssh/sovkit", "/home/alice/.ssh/sovkit_known_hosts")
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o, want 600", info.Mode().Perm())
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != cfg {
		t.Fatalf("loaded config = %#v, want %#v", loaded, cfg)
	}
}
