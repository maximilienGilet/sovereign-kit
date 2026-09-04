package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestInstallerOnlyInstallsCLIAndPreservesProfiles(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, "test-bin")
	os.Mkdir(bin, 0700)
	// Fake only the external build boundary; never install packages or touch real HOME.
	os.WriteFile(filepath.Join(bin, "go"), []byte("#!/bin/sh\nwhile [ $# -gt 0 ]; do if [ \"$1\" = -o ]; then shift; printf '#!/bin/sh\\nexit 0\\n' > \"$1\"; exit 0; fi; shift; done\nexit 1\n"), 0700)
	for _, name := range []string{"pi", "npm"} {
		os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nprintf forbidden > \"$HOME/forbidden\"\nexit 1\n"), 0700)
	}
	profile := filepath.Join(home, ".pi/profiles/sovereign/agent")
	os.MkdirAll(profile, 0700)
	settings := filepath.Join(profile, "settings.json")
	os.WriteFile(settings, []byte("existing user data"), 0600)
	script, err := filepath.Abs("../../install-macos.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/bin/bash", script, "--upgrade")
	cmd.Env = []string{"HOME=" + home, "PATH=" + bin + ":/usr/bin:/bin"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI installer failed: %v %s", err, out)
	}
	raw, _ := os.ReadFile(settings)
	if string(raw) != "existing user data" {
		t.Fatal("installer modified profile")
	}
	if _, err := os.Stat(filepath.Join(home, "forbidden")); !os.IsNotExist(err) {
		t.Fatal("installer ran client/package installation")
	}
	if _, err := os.Stat(filepath.Join(home, ".local/bin/sovkit")); err != nil {
		t.Fatal("CLI missing")
	}
}
