package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
)

type ManualRoute struct {
	Host           string
	Port           int
	User           string
	IdentityFile   string
	KnownHostsFile string
}

type SetupPrompter interface {
	SelectProvider(context.Context) (string, error)
	ManualRoute(context.Context, string) (ManualRoute, error)
	VastIdentity(context.Context, string) (string, error)
}

type SetupDependencies struct {
	Prompter SetupPrompter
	Getenv   func(string) string
	HomeDir  func() (string, error)
	RunVast  func(context.Context, string, string, setup.Operator) (setup.Result, error)
}

func Setup(ctx context.Context, input io.Reader, output io.Writer, configPath, defaultUser string, deps SetupDependencies) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if deps.Prompter == nil {
		deps.Prompter = NewHuhPrompter(input, output)
	}
	if deps.Getenv == nil {
		deps.Getenv = os.Getenv
	}
	if deps.HomeDir == nil {
		deps.HomeDir = os.UserHomeDir
	}
	provider, err := deps.Prompter.SelectProvider(ctx)
	if err != nil {
		return err
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	fmt.Fprintln(output, "Sovereign Kit setup")
	switch provider {
	case "manual":
		return setupManual(ctx, output, configPath, defaultUser, deps.Prompter)
	case "vast":
		return setupVast(ctx, output, deps)
	default:
		return fmt.Errorf("unknown setup provider %q", provider)
	}
}

func setupManual(ctx context.Context, output io.Writer, configPath, defaultUser string, prompter SetupPrompter) error {
	route, err := prompter.ManualRoute(ctx, defaultUser)
	if err != nil {
		return err
	}
	if route.Port < 1 || route.Port > 65535 {
		return fmt.Errorf("SSH port must be between 1 and 65535")
	}
	for label, path := range map[string]string{"SSH identity file": route.IdentityFile, "verified known-hosts file": route.KnownHostsFile} {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			return fmt.Errorf("%s is not a readable file: %s", label, path)
		}
	}
	if err := config.Save(configPath, config.Studio(route.Host, route.Port, route.User, route.IdentityFile, route.KnownHostsFile)); err != nil {
		return err
	}
	fmt.Fprintf(output, "Configuration saved: %s\n", configPath)
	fmt.Fprintln(output, "Next: sovkit tunnel, then sovkit doctor.")
	return nil
}

func setupVast(ctx context.Context, output io.Writer, deps SetupDependencies) error {
	token := deps.Getenv("VAST_API_KEY")
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("Vast API key is required")
	}
	if deps.RunVast == nil {
		return fmt.Errorf("Vast runner is required")
	}
	home, err := deps.HomeDir()
	if err != nil {
		return fmt.Errorf("find home directory: %w", err)
	}
	identity, err := deps.Prompter.VastIdentity(ctx, filepath.Join(home, ".ssh", "id_ed25519"))
	if err != nil {
		return err
	}
	operator, ok := deps.Prompter.(setup.Operator)
	if !ok {
		return fmt.Errorf("Vast setup prompter must implement setup operator")
	}
	result, err := deps.RunVast(ctx, token, identity, operator)
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "Vast instance %d created.\n", result.InstanceID)
	fmt.Fprintln(output, "Warning: billing may still be active for this instance.")
	fmt.Fprintln(output, "Next: sovkit start")
	return nil
}
