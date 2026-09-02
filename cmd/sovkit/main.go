package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/maximilienGilet/sovereign-kit/internal/catalogui"
	"github.com/maximilienGilet/sovereign-kit/internal/cli"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/route"
)

func main() {
	path, err := defaultConfigPath()
	if err == nil {
		err = runWith(os.Args[1:], os.Stdin, os.Stdout, path, os.Getenv("USER"))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "sovkit:", err)
		os.Exit(2)
	}
}

func run(args []string, output io.Writer) error {
	path, err := defaultConfigPath()
	if err != nil {
		return err
	}
	return runWith(args, os.Stdin, output, path, os.Getenv("USER"))
}

func runWith(args []string, input io.Reader, output io.Writer, configPath, defaultUser string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(output, `Usage: sovkit <command>

Commands:
  setup       Save a verified private SSH route configuration
  catalog     Browse recipes and inspect their requirements
  dashboard   Open the recipe dashboard
  tunnel      Start the private SSH loopback tunnel
  doctor      Check the local route health`)
		return err
	}
	switch args[0] {
	case "setup":
		return cli.Setup(input, output, configPath, defaultUser)
	case "catalog", "dashboard":
		_, err := tea.NewProgram(catalogui.New(catalogui.DefaultEntries()), tea.WithOutput(output)).Run()
		return err
	case "tunnel":
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		command, err := route.Command(cfg)
		if err != nil {
			return err
		}
		command.Stdout, command.Stderr = output, output
		return command.Run()
	case "doctor":
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		endpoint := fmt.Sprintf("http://%s:%d", cfg.Route.LocalHost, cfg.Route.LocalPort)
		if err := route.Healthcheck(ctx, endpoint); err != nil {
			return fmt.Errorf("local route %s is unavailable: %w", endpoint, err)
		}
		_, err = fmt.Fprintf(output, "PASS  local route answered %s/v1/models\n", endpoint)
		return err
	default:
		return fmt.Errorf("unknown command %q (try: sovkit help)", args[0])
	}
}

func defaultConfigPath() (string, error) {
	if configured := os.Getenv("SOVKIT_CONFIG"); configured != "" {
		return configured, nil
	}
	home, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return config.Path(filepath.Clean(home)), nil
}
