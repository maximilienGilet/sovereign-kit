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
	"github.com/maximilienGilet/sovereign-kit/internal/sshkey"
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

// TunnelSpec describes one SSH loopback forward for a deployment. Both
// sides stay on loopback; only Destroyed records release their local port.
type TunnelSpec struct {
	SSHHost        string
	SSHPort        int
	SSHUser        string
	IdentityFile   string
	KnownHostsFile string
	LocalHost      string
	LocalPort      int
	RemoteHost     string
	RemotePort     int
}

// ForwardCommand builds the SSH process for one deployment forward.
func ForwardCommand(ctx context.Context, spec TunnelSpec) (*exec.Cmd, error) {
	if strings.TrimSpace(spec.SSHHost) == "" {
		return nil, fmt.Errorf("SSH host is required")
	}
	if spec.SSHPort < 1 || spec.SSHPort > 65535 {
		return nil, fmt.Errorf("SSH port must be between 1 and 65535")
	}
	if strings.TrimSpace(spec.SSHUser) == "" {
		return nil, fmt.Errorf("SSH user is required")
	}
	if spec.LocalHost != "127.0.0.1" || spec.RemoteHost != "127.0.0.1" {
		return nil, fmt.Errorf("tunnel must use loopback (127.0.0.1) on both sides")
	}
	if spec.LocalPort < 1 || spec.LocalPort > 65535 || spec.RemotePort < 1 || spec.RemotePort > 65535 {
		return nil, fmt.Errorf("tunnel ports must be between 1 and 65535")
	}
	if err := requireReadableRegularFile("SSH identity file", spec.IdentityFile); err != nil {
		return nil, err
	}
	if err := requireReadableRegularFile("SSH known hosts file", spec.KnownHostsFile); err != nil {
		return nil, err
	}
	return exec.CommandContext(
		ctx,
		"ssh",
		"-N",
		"-o", "BatchMode=yes",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "IdentitiesOnly=yes",
		"-o", "StrictHostKeyChecking=yes",
		"-o", sshkey.KnownHostsOption(spec.KnownHostsFile),
		"-i", spec.IdentityFile,
		"-L", fmt.Sprintf("%s:%d:%s:%d", spec.LocalHost, spec.LocalPort, spec.RemoteHost, spec.RemotePort),
		"-p", strconv.Itoa(spec.SSHPort),
		fmt.Sprintf("%s@%s", spec.SSHUser, spec.SSHHost),
	), nil
}

// Command creates the SSH process for cfg's fixed loopback forward.
func Command(cfg config.Config) (*exec.Cmd, error) {
	return CommandContext(context.Background(), cfg)
}

func CommandContext(ctx context.Context, cfg config.Config) (*exec.Cmd, error) {
	return ForwardCommand(ctx, TunnelSpec{
		SSHHost: cfg.SSH.Host, SSHPort: cfg.SSH.Port, SSHUser: cfg.SSH.User,
		IdentityFile: cfg.SSH.IdentityFile, KnownHostsFile: cfg.SSH.KnownHostsFile,
		LocalHost: cfg.Route.LocalHost, LocalPort: cfg.Route.LocalPort,
		RemoteHost: cfg.Route.RemoteHost, RemotePort: cfg.Route.RemotePort,
	})
}
