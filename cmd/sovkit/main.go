package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/cli"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/route"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
)

type doctorExitError struct {
	code int
}

func (err *doctorExitError) Error() string {
	return fmt.Sprintf("doctor exited with status %d", err.code)
}

func main() {
	path, err := defaultConfigPath()
	if err == nil {
		err = runWith(os.Args[1:], os.Stdout, path)
	}
	if err != nil {
		var doctorExit *doctorExitError
		if errors.As(err, &doctorExit) {
			os.Exit(doctorExit.code)
		}
		fmt.Fprintln(os.Stderr, "sovkit:", err)
		os.Exit(2)
	}
}

func run(args []string, output io.Writer) error {
	path, err := defaultConfigPath()
	if err != nil {
		return err
	}
	return runWith(args, output, path)
}

func runWith(args []string, output io.Writer, configPath string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(output, `Usage: sovkit <command>

Commands:
  start       Connect the private tunnel and wait for a healthy route
  tunnel      Start the private SSH loopback tunnel
  doctor      Check the local route health`)
		return err
	}
	switch args[0] {
	case "start":
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		return cli.StartHeadless(ctx, output, configPath, cli.StartDependencies{
			Healthcheck:  route.Healthcheck,
			Clock:        setup.RealClock{},
			PollInterval: 5 * time.Second,
			PollTimeout:  30 * time.Minute,
		})
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
		if handled, err := runInstalledDoctor(output); handled {
			return err
		}
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

func runInstalledDoctor(output io.Writer) (bool, error) {
	executable, err := os.Executable()
	if err != nil {
		return false, nil
	}
	helper := filepath.Join(filepath.Dir(executable), "sovkit-doctor")
	info, err := os.Stat(helper)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return false, nil
	}
	command := exec.Command(helper, "doctor")
	command.Stdout = output
	command.Stderr = output
	if err := command.Run(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return true, &doctorExitError{code: exitError.ExitCode()}
		}
		return true, err
	}
	return true, nil
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
