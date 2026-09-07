package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestVastKeySurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vast-api-key")
	store := FileVastCredentials{Path: path}
	first := &fakeSetupPrompter{apiKey: " synthetic-secret "}
	deps := SetupDependencies{Prompter: first, Getenv: func(string) string { return "" }, Credentials: store}
	token, err := vastAPIKey(context.Background(), deps)
	if err != nil || token != "synthetic-secret" {
		t.Fatalf("first key acquisition failed: %v", err)
	}
	second := &fakeSetupPrompter{}
	deps.Prompter = second
	deps.Credentials = FileVastCredentials{Path: path}
	token, err = vastAPIKey(context.Background(), deps)
	if err != nil || token != "synthetic-secret" || second.apiKeyCalls != 0 {
		t.Fatal("restart did not reuse saved key")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("credential permissions are unsafe")
	}
	deps.Getenv = func(string) string { return "environment-key" }
	token, err = vastAPIKey(context.Background(), deps)
	if err != nil || token != "environment-key" {
		t.Fatal("environment override lost")
	}
	saved, _ := store.Load()
	if saved != "synthetic-secret" {
		t.Fatal("environment key persisted without request")
	}
}

func TestCredentialStoreRejectsUnsafeFiles(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := (FileVastCredentials{Path: target}).Load(); err == nil {
		t.Fatal("read world-readable secret")
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := (FileVastCredentials{Path: link}).Load(); err == nil {
		t.Fatal("followed symlink")
	}
}
