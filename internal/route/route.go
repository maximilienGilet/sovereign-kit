// Package route creates and checks Sovereign Kit's local SSH route.
package route

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
)

// Healthcheck confirms that the local, unauthenticated V1 inference route responds.
func Healthcheck(ctx context.Context, baseURL string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/v1/models", nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("healthcheck returned %s", response.Status)
	}
	return nil
}

func requireReadableRegularFile(label, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", label, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", label, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a regular file", label)
	}
	return nil
}

// Command creates the SSH process for cfg's fixed loopback forward.
func Command(cfg config.Config) (*exec.Cmd, error) {
	if err := requireReadableRegularFile("SSH identity file", cfg.SSH.IdentityFile); err != nil {
		return nil, err
	}
	if err := requireReadableRegularFile("SSH known hosts file", cfg.SSH.KnownHostsFile); err != nil {
		return nil, err
	}

	return exec.Command(
		"ssh",
		"-N",
		"-o", "BatchMode=yes",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "IdentitiesOnly=yes",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UserKnownHostsFile="+cfg.SSH.KnownHostsFile,
		"-i", cfg.SSH.IdentityFile,
		"-L", "127.0.0.1:30000:127.0.0.1:30000",
		"-p", strconv.Itoa(cfg.SSH.Port),
		fmt.Sprintf("%s@%s", cfg.SSH.User, cfg.SSH.Host),
	), nil
}
