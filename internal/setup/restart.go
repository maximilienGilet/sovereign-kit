package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/recipes"
)

// RefreshSavedHostTrust pins the same public keys at a refreshed SSH address.
// A changed key is never silently accepted during restart.
func RefreshSavedHostTrust(ctx context.Context, old, next config.SSH) error {
	keys, err := waitForHostKeys(ctx, next.Host, next.Port, 0, Dependencies{Clock: RealClock{}, HostKeyScanner: SystemHostKeyScanner{Runner: ExecRunner{}}})
	if err != nil {
		return err
	}
	previous, err := os.ReadFile(old.KnownHostsFile)
	if err != nil {
		return err
	}
	if !sameHostPublicKeys(previous, keys.Raw) {
		return fmt.Errorf("SSH host keys changed; refusing automatic trust on restart")
	}
	return (FileTrustStore{}).Save(next.KnownHostsFile, keys.Raw)
}

func sameHostPublicKeys(old, next []byte) bool {
	parse := func(data []byte) map[string]bool {
		keys := map[string]bool{}
		for _, line := range strings.Split(string(data), "\n") {
			f := strings.Fields(line)
			if len(f) >= 3 && !strings.HasPrefix(f[0], "#") {
				keys[f[1]+" "+f[2]] = true
			}
		}
		return keys
	}
	a, b := parse(old), parse(next)
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	for key := range b {
		if !a[key] {
			return false
		}
	}
	return true
}

// RestartSavedServer recovers the exact pinned recipe, never guesses from model ID.
func RestartSavedServer(ctx context.Context, cfg config.Config) (config.Config, error) {
	// The caller supplies fresh coordinates, while the known-hosts file still
	// carries the previously approved keys. Validate these before any remote command.
	if err := RefreshSavedHostTrust(ctx, cfg.SSH, cfg.SSH); err != nil {
		return cfg, err
	}
	runner := ExecRunner{}
	readCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	output, err := runner.Output(readCtx, strictSSHCommand(cfg.SSH, "cat /workspace/.sovkit-deployment"))
	cancel()
	if err != nil {
		return cfg, err
	}
	var fingerprint string
	if err := json.Unmarshal(output, &fingerprint); err != nil {
		return cfg, fmt.Errorf("read deployment fingerprint: %w", err)
	}
	candidates, err := recipes.Builtin()
	if err != nil {
		return cfg, err
	}
	if cfg.DeploymentRecipe != "" {
		var saved recipe.Recipe
		if err := json.Unmarshal([]byte(cfg.DeploymentRecipe), &saved); err != nil {
			return cfg, err
		}
		candidates = append([]recipe.Recipe{saved}, candidates...)
	}
	for _, candidate := range candidates {
		snapshot, err := json.Marshal(candidate)
		if err != nil {
			return cfg, err
		}
		if digest(snapshot) != fingerprint {
			continue
		}
		if err := candidate.Validate(); err != nil {
			return cfg, err
		}
		if err := (StrictSSHLauncher{Runner: runner}).Restart(ctx, cfg.SSH, candidate); err != nil {
			return cfg, err
		}
		cfg.DeploymentRecipe = string(snapshot)
		return cfg, nil
	}
	return cfg, fmt.Errorf("cannot identify the exact saved recipe; refusing to launch different model settings")
}
