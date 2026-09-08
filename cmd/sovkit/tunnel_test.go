package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/maximilienGilet/sovereign-kit/internal/state"
)

func helperStore() state.Store {
	return state.Store{
		Version:  1,
		Settings: state.DefaultSettings(),
		Deployments: []state.Deployment{
			{ID: "live-one", State: state.Serving, Route: state.Route{LocalHost: "127.0.0.1", LocalPort: 30000, RemoteHost: "127.0.0.1", RemotePort: 30000}},
			{ID: "dead-one", State: state.Destroyed, Route: state.Route{LocalHost: "127.0.0.1", LocalPort: 30001, RemoteHost: "127.0.0.1", RemotePort: 30000}},
		},
		Active: "live-one",
	}
}

func TestResolveDeploymentFindsByIDAndActive(t *testing.T) {
	store := helperStore()
	deployment, err := resolveDeployment(t.TempDir(), store, "dead-one")
	if err != nil || deployment.ID != "dead-one" {
		t.Fatalf("by id = %+v, %v", deployment, err)
	}
	deployment, err = resolveDeployment(t.TempDir(), store, "")
	if err != nil || deployment.ID != "live-one" {
		t.Fatalf("active = %+v, %v", deployment, err)
	}
	if _, err := resolveDeployment(t.TempDir(), store, "missing"); err == nil {
		t.Fatal("expected unknown deployment error")
	}
	var empty state.Store
	if _, err := resolveDeployment(t.TempDir(), empty, ""); err == nil {
		t.Fatal("expected no-active error")
	}
}

func TestReadConfirmLineTreatsUnreadableAsEmpty(t *testing.T) {
	if got := readConfirmLine(strings.NewReader("y\n")); got != "y" {
		t.Fatalf("got %q", got)
	}
	if got := readConfirmLine(strings.NewReader("  YES  \n")); got != "yes" {
		t.Fatalf("got %q", got)
	}
	if got := readConfirmLine(strings.NewReader("")); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestStdinInteractiveDetectsTerminal(t *testing.T) {
	if stdinInteractive(strings.NewReader("")) {
		t.Fatal("strings reader is never a terminal")
	}
	previous := stdinInteractive
	t.Cleanup(func() { stdinInteractive = previous })
	stdinInteractive = func(io.Reader) bool { return true }
	if !stdinInteractive(os.Stdin) {
		t.Fatal("expected terminal with override")
	}
	stdinInteractive = func(io.Reader) bool { return false }
	if stdinInteractive(os.Stdin) {
		t.Fatal("expected non-terminal with override")
	}
}
