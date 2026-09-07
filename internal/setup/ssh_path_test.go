package setup

import (
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/route"
	"os"
	"path/filepath"
	"testing"
)

func TestSSHCommandsQuoteApprovedHostKeyPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Application Support")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	hosts := filepath.Join(dir, "known_hosts")
	identity := filepath.Join(dir, "identity")
	for _, path := range []string{hosts, identity} {
		if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Studio("example.invalid", 34278, "fixture", identity, hosts)
	tunnel, err := route.Command(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for kind, args := range map[string][]string{"launcher": strictSSHCommand(cfg.SSH, "true").Args, "tunnel": tunnel.Args} {
		want := `UserKnownHostsFile="` + hosts + `"`
		found, strict := false, false
		for _, arg := range args {
			if arg == want {
				found = true
			}
			if arg == "StrictHostKeyChecking=yes" {
				strict = true
			}
		}
		if !found || !strict {
			t.Fatalf("%s must quote the SSH config value without disabling verification: %q", kind, args)
		}
	}
}
