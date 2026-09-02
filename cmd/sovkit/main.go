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
	"github.com/maximilienGilet/sovereign-kit/recipes"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/route"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

const vastBaseURL = "https://console.vast.ai"

type application struct {
	setup func(io.Reader, io.Writer, string, string) error
	start func(io.Writer, string) error
}

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

func runWith(args []string, input io.Reader, output io.Writer, configPath, defaultUser string, apps ...application) error {
	app := productionApplication()
	if len(apps) > 0 {
		if apps[0].setup != nil {
			app.setup = apps[0].setup
		}
		if apps[0].start != nil {
			app.start = apps[0].start
		}
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(output, `Usage: sovkit <command>

Commands:
  setup       Provision a verified private SSH route
  start       Start the private tunnel and dashboard
  catalog     Browse recipes and inspect their requirements
  dashboard   Open the recipe dashboard
  tunnel      Start the private SSH loopback tunnel
  doctor      Check the local route health`)
		return err
	}
	switch args[0] {
	case "setup":
		return app.setup(input, output, configPath, defaultUser)
	case "start":
		return app.start(output, configPath)
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

type productionDependencies struct {
	Prompter cli.SetupPrompter
	Getenv   func(string) string
	HomeDir  func() (string, error)
	RunVast  func(context.Context, string, recipe.Recipe, setup.Options, setup.Dependencies) (setup.Result, error)
}

func productionApplication() application {
	return productionApplicationWith(productionDependencies{
		Getenv:  os.Getenv,
		HomeDir: os.UserHomeDir,
		RunVast: setup.RunVast,
	})
}

func productionApplicationWith(deps productionDependencies) application {
	if deps.Getenv == nil {
		deps.Getenv = os.Getenv
	}
	if deps.HomeDir == nil {
		deps.HomeDir = os.UserHomeDir
	}
	if deps.RunVast == nil {
		deps.RunVast = setup.RunVast
	}
	return application{
		setup: func(input io.Reader, output io.Writer, configPath, defaultUser string) error {
			recipe, err := recipes.QwenStudio()
			if err != nil {
				return fmt.Errorf("load Qwen Studio recipe: %w", err)
			}
			return cli.Setup(context.Background(), input, output, configPath, defaultUser, cli.SetupDependencies{
				Prompter: deps.Prompter,
				Getenv:   deps.Getenv,
				HomeDir:  deps.HomeDir,
				RunVast: func(ctx context.Context, token, identity string, operator setup.Operator) (setup.Result, error) {
					return deps.RunVast(ctx, token, recipe, setup.Options{
						ConfigPath:    configPath,
						IdentityFile:  identity,
						KnownHostsDir: filepath.Join(filepath.Dir(configPath), "known_hosts"),
						OfferLimit:    5,
						PollInterval:  5 * time.Second,
						PollTimeout:   10 * time.Minute,
					}, setup.Dependencies{
						NewAPI: func(token string) setup.VastAPI {
							return vast.NewClient(vastBaseURL, token)
						},
						Operator:         operator,
						HostKeyScanner:   setup.SystemHostKeyScanner{Runner: setup.ExecRunner{}},
						TrustStore:       setup.FileTrustStore{},
						ServerLauncher:   setup.StrictSSHLauncher{Runner: setup.ExecRunner{}},
						Clock:            setup.RealClock{},
						SaveConfig:       config.Save,
						ValidateIdentity: setup.ValidateIdentityFile,
					})
				},
			})
		},
		start: func(output io.Writer, configPath string) error {
			return cli.Start(context.Background(), output, configPath, cli.StartDependencies{
				Healthcheck:  route.Healthcheck,
				RunDashboard: func(output io.Writer) error {
					_, err := tea.NewProgram(catalogui.New(catalogui.DefaultEntries()), tea.WithOutput(output)).Run()
					return err
				},
				Clock:        setup.RealClock{},
				PollInterval: 5 * time.Second,
				PollTimeout:  30 * time.Minute,
			})
		},
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
