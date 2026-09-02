package config

import (
	"path/filepath"
	"testing"
)

func TestPathPlacesConfigUnderSovereignKit(t *testing.T) {
	got := Path("/tmp/config-home")
	want := filepath.Join("/tmp/config-home", "sovereign-kit", "config.toml")
	if got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}
