// Package config owns the one local Sovereign Kit configuration file.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const StudioProfile = "qwen-studio"

type Config struct {
	Version  int      `toml:"version"`
	Profile  string   `toml:"profile"`
	Provider Provider `toml:"provider"`
	Route    Route    `toml:"route"`
	SSH      SSH      `toml:"ssh"`
}

type Provider struct {
	Kind       string `toml:"kind"`
	InstanceID int    `toml:"instance_id"`
}

type Route struct {
	LocalHost  string `toml:"local_host"`
	LocalPort  int    `toml:"local_port"`
	RemoteHost string `toml:"remote_host"`
	RemotePort int    `toml:"remote_port"`
}

type SSH struct {
	Host           string `toml:"host"`
	Port           int    `toml:"port"`
	User           string `toml:"user"`
	IdentityFile   string `toml:"identity_file"`
	KnownHostsFile string `toml:"known_hosts_file"`
}

// Studio returns the only V1 route: Qwen through an SSH loopback forward.
func Studio(host string, port int, user, identityFile, knownHostsFile string) Config {
	return Config{
		Version: 1,
		Profile: StudioProfile,
		Provider: Provider{Kind: "manual"},
		Route: Route{
			LocalHost: "127.0.0.1", LocalPort: 30000,
			RemoteHost: "127.0.0.1", RemotePort: 30000,
		},
		SSH: SSH{Host: host, Port: port, User: user, IdentityFile: identityFile, KnownHostsFile: knownHostsFile},
	}
}

func Load(path string) (Config, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := toml.Unmarshal(contents, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	contents, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".config.*.toml")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func (cfg Config) Validate() error {
	if cfg.Version != 1 || cfg.Profile != StudioProfile {
		return fmt.Errorf("only the %q profile at version 1 is supported", StudioProfile)
	}
	if cfg.Provider.Kind != "manual" && cfg.Provider.Kind != "vast" {
		return fmt.Errorf("provider kind must be manual or vast")
	}
	if cfg.Provider.InstanceID < 0 {
		return fmt.Errorf("provider instance_id cannot be negative")
	}
	if cfg.Route.LocalHost != "127.0.0.1" || cfg.Route.RemoteHost != "127.0.0.1" {
		return fmt.Errorf("the inference route must use loopback (127.0.0.1) on both sides")
	}
	if cfg.Route.LocalPort != 30000 || cfg.Route.RemotePort != 30000 {
		return fmt.Errorf("the V1 route uses port 30000 on both sides")
	}
	if strings.TrimSpace(cfg.SSH.Host) == "" || strings.ContainsAny(cfg.SSH.Host, " \t\n") {
		return fmt.Errorf("SSH host is required and cannot contain whitespace")
	}
	if cfg.SSH.Port < 1 || cfg.SSH.Port > 65535 {
		return fmt.Errorf("SSH port must be between 1 and 65535")
	}
	if strings.TrimSpace(cfg.SSH.User) == "" || strings.ContainsAny(cfg.SSH.User, " \t\n") {
		return fmt.Errorf("SSH user is required and cannot contain whitespace")
	}
	if strings.TrimSpace(cfg.SSH.IdentityFile) == "" || strings.TrimSpace(cfg.SSH.KnownHostsFile) == "" {
		return fmt.Errorf("SSH identity_file and known_hosts_file are required")
	}
	return nil
}
