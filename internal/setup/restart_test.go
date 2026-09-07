package setup

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplicitRestartUsesMatchingDeploymentOnly(t *testing.T) {
	for _, match := range []bool{true, false} {
		dir := t.TempDir()
		proc := filepath.Join(dir, "proc")
		os.Mkdir(proc, 0700)
		marker := filepath.Join(dir, "deployment")
		calls := filepath.Join(dir, "calls")
		snapshot, _ := json.Marshal(validRecipe())
		hash := digest(snapshot)
		if !match {
			hash = "foreign"
		}
		data, _ := json.Marshal(hash)
		os.WriteFile(marker, data, 0600)
		// The explicit restart mode must be opt-in; ordinary resume stays fail-closed.
		script := restartReconciliationScript(validRecipe(), "echo dispatched >> "+shellQuote(calls), marker, proc)
		script = strings.Replace(script, "probe = socket.socket()", "probe = type('FreePort', (), {'bind': lambda self, address: None, 'close': lambda self: None})()", 1)
		out, err := exec.CommandContext(context.Background(), "python3", "-c", script).CombinedOutput()
		if (err == nil) != match {
			t.Fatalf("matching=%v: %v %s", match, err, out)
		}
		_, err = os.Stat(calls)
		if (err == nil) != match {
			t.Fatal("unsafe/missing launch")
		}
	}
}

func TestRefreshRequiresPreviouslyPinnedPublicKey(t *testing.T) {
	old := []byte("[old]:22 ssh-ed25519 pinned-key\n")
	for _, tc := range []struct {
		next string
		want bool
	}{{"[new]:123 ssh-ed25519 pinned-key\n", true}, {"[new]:123 ssh-ed25519 foreign-key\n", false}, {"", false}, {"# comment\n", false}} {
		if sameHostPublicKeys(old, []byte(tc.next)) != tc.want {
			t.Fatalf("wrong trust result for %q", tc.next)
		}
	}
}
