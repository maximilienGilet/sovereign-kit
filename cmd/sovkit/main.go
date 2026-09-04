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
	"strconv"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/maximilienGilet/sovereign-kit/internal/catalogui"
	"github.com/maximilienGilet/sovereign-kit/internal/cli"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/huggingface"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/route"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
	"github.com/maximilienGilet/sovereign-kit/recipes"
)

const vastBaseURL = "https://console.vast.ai"
const huggingFaceBaseURL = "https://huggingface.co"

type application struct {
	setup       func(io.Reader, io.Writer, string, string) error
	start       func(io.Writer, string) error
	dashboard   func(io.Reader, io.Writer) error
	interactive func(io.Reader, io.Writer) bool
	terminal    func(context.Context, io.Reader, io.Writer, string, string, string) error
	resume      func(io.Reader, io.Writer, string, string, int) error
}

type doctorExitError struct {
	code int
}

func (err *doctorExitError) Error() string {
	return fmt.Sprintf("doctor exited with status %d", err.code)
}

func main() {
	path, err := defaultConfigPath()
	if err == nil {
		err = runWith(os.Args[1:], os.Stdin, os.Stdout, path, os.Getenv("USER"))
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
		if apps[0].dashboard != nil {
			app.dashboard = apps[0].dashboard
		}
		if apps[0].interactive != nil {
			app.interactive = apps[0].interactive
		}
		if apps[0].terminal != nil {
			app.terminal = apps[0].terminal
		}
		if apps[0].resume != nil {
			app.resume = apps[0].resume
		}
	}
	helpRequested := len(args) > 0 && args[0] == "help"
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			helpRequested = true
		}
	}
	interactive := app.interactive(input, output)
	resumeID := 0
	if !helpRequested && len(args) > 0 && args[0] == "resume" {
		if len(args) > 2 {
			return fmt.Errorf("usage: sovkit resume [INSTANCE_ID]")
		}
		if len(args) == 2 {
			var err error
			resumeID, err = strconv.Atoi(args[1])
			if err != nil || resumeID <= 0 {
				return fmt.Errorf("instance ID must be a positive integer")
			}
		}
		if interactive {
			entry := "resume"
			if resumeID > 0 {
				entry += ":" + strconv.Itoa(resumeID)
			}
			return app.terminal(context.Background(), input, output, configPath, defaultUser, entry)
		}
		return app.resume(input, output, configPath, defaultUser, resumeID)
	}
	if !helpRequested && interactive && (len(args) == 0 || args[0] == "setup" || args[0] == "start") {
		entry := "home"
		if len(args) > 0 {
			entry = args[0]
		}
		// Bubble Tea owns SIGINT, including releasing it to launched clients.
		return app.terminal(context.Background(), input, output, configPath, defaultUser, entry)
	}
	if len(args) == 0 || helpRequested {
		_, err := fmt.Fprintln(output, `Usage: sovkit [command]

Without a command, open the terminal application (or show help outside a terminal).

Commands:
  setup       Provision a verified private SSH route
  resume [ID] Resume an interrupted deployment or recover an existing instance
  start       Connect the private tunnel (dashboard on interactive terminals)
  catalog     Browse recipes and inspect their requirements
  dashboard   Connect and open the private endpoint dashboard
  tunnel      Start the private SSH loopback tunnel
  doctor      Check the local route health`)
		return err
	}
	switch args[0] {
	case "setup":
		return app.setup(input, output, configPath, defaultUser)
	case "start":
		if _, err := setup.ReadCheckpoint(setup.CheckpointPath(configPath)); !os.IsNotExist(err) {
			if err != nil {
				return fmt.Errorf("pending deployment: %w", err)
			}
			return app.resume(input, output, configPath, defaultUser, 0)
		}
		return app.start(output, configPath)
	case "dashboard":
		return app.dashboard(input, output)
	case "catalog":
		if !interactive {
			for _, entry := range catalogui.DefaultEntries() {
				if _, err := fmt.Fprintf(output, "%s · %s\n%s\n\n", entry.Name, entry.Status, entry.Summary); err != nil {
					return err
				}
			}
			return nil
		}
		_, err := tea.NewProgram(catalogui.New(catalogui.DefaultEntries()), tea.WithInput(input), tea.WithOutput(output)).Run()
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

type productionDependencies struct {
	Prompter        cli.SetupPrompter
	Getenv          func(string) string
	HomeDir         func() (string, error)
	LoadRecipes     func() ([]recipe.Recipe, error)
	SearchModels    func(context.Context, string, int) ([]huggingface.SearchResult, error)
	InspectModel    func(context.Context, string) (huggingface.Model, error)
	PrepareIdentity func(context.Context, string, string, setup.IdentityOperator) error
	RunVast         func(context.Context, string, recipe.Recipe, setup.Options, setup.Dependencies) (setup.Result, error)
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
	if deps.LoadRecipes == nil {
		deps.LoadRecipes = recipes.Builtin
	}
	huggingFaceClient := huggingface.NewClient(huggingFaceBaseURL)
	if deps.SearchModels == nil {
		deps.SearchModels = huggingFaceClient.Search
	}
	if deps.InspectModel == nil {
		deps.InspectModel = huggingFaceClient.Inspect
	}
	if deps.RunVast == nil {
		deps.RunVast = setup.RunVast
	}
	if deps.PrepareIdentity == nil {
		deps.PrepareIdentity = func(ctx context.Context, token, path string, operator setup.IdentityOperator) error {
			return setup.PrepareVastIdentity(ctx, path, vast.NewClient(vastBaseURL, token), setup.ExecRunner{}, operator)
		}
	}
	setupDependencies := func(configPath string) cli.SetupDependencies {
		return cli.SetupDependencies{
			Credentials:  cli.FileVastCredentials{Path: configPath + ".vast-api-key"},
			Prompter:     deps.Prompter,
			Getenv:       deps.Getenv,
			HomeDir:      deps.HomeDir,
			LoadRecipes:  deps.LoadRecipes,
			SearchModels: deps.SearchModels,
			InspectModel: deps.InspectModel,
			RunVast: func(ctx context.Context, token, identity string, workload cli.VastWorkload, operator setup.Operator) (setup.Result, error) {
				identityOperator, ok := operator.(setup.IdentityOperator)
				if !ok {
					return setup.Result{}, fmt.Errorf("Vast setup prompter must confirm SSH identity setup")
				}
				return deps.RunVast(ctx, token, workload.Recipe, setup.Options{
					ConfigPath:       configPath,
					IdentityFile:     identity,
					KnownHostsDir:    filepath.Join(filepath.Dir(configPath), "known_hosts"),
					AutoSelectOffer:  workload.AutoSelectOffer,
					OfferLimit:       100,
					PollInterval:     5 * time.Second,
					PollTimeout:      10 * time.Minute,
					CheckpointPath:   setup.CheckpointPath(configPath),
					Resume:           workload.Resume,
					ResumeInstanceID: workload.ResumeInstanceID,
					RecoveryOnly:     workload.RecoveryOnly,
				}, setup.Dependencies{
					NewAPI: func(token string) setup.VastAPI {
						return vast.NewClient(vastBaseURL, token)
					},
					Operator:       operator,
					HostKeyScanner: setup.SystemHostKeyScanner{Runner: setup.ExecRunner{}},
					TrustStore:     setup.FileTrustStore{},
					ServerLauncher: setup.StrictSSHLauncher{Runner: setup.ExecRunner{}, LogSecrets: []string{token}, OnLogs: func(logs setup.ServerLogs) {
						if observer, ok := operator.(setup.ServerLogObserver); ok {
							observer.ServerLogSnapshot(logs)
						}
					}},
					Clock:      setup.RealClock{},
					SaveConfig: config.Save,
					PrepareIdentity: func(ctx context.Context) error {
						return deps.PrepareIdentity(ctx, token, identity, identityOperator)
					},
					ValidateIdentity: setup.ValidateIdentityFile,
				})
			},
		}
	}
	startDependencies := cli.StartDependencies{
		Healthcheck: route.Healthcheck, Clock: setup.RealClock{}, PollInterval: 5 * time.Second, PollTimeout: 30 * time.Minute,
	}
	return application{
		resume: func(input io.Reader, output io.Writer, configPath, defaultUser string, id int) error {
			restore := guardLegacyTerminal(input)
			defer restore()
			resumeDeps := setupDependencies(configPath)
			if resumeDeps.Prompter == nil {
				resumeDeps.Prompter = cli.NewAccessiblePrompter(input, output)
			}
			return cli.ResumeSetup(context.Background(), output, configPath, resumeDeps, id, false)
		},
		interactive: func(input io.Reader, output io.Writer) bool {
			return cli.UseTerminalApplication(input, output, deps.Getenv)
		},
		terminal: func(ctx context.Context, input io.Reader, output io.Writer, configPath, defaultUser, entry string) error {
			return cli.RunApplication(ctx, input, output, configPath, defaultUser, entry, cli.ApplicationDependencies{Setup: setupDependencies(configPath), Start: startDependencies})
		},
		setup: func(input io.Reader, output io.Writer, configPath, defaultUser string) error {
			restore := guardLegacyTerminal(input)
			defer restore()
			// Lstat also detects invalid configs and dangling symlinks. Consent
			// must precede Setup, since it may create paid remote resources.
			_, pendingErr := os.Lstat(setup.CheckpointPath(configPath))
			if _, err := os.Lstat(configPath); !os.IsNotExist(err) && os.IsNotExist(pendingErr) {
				confirmed, err := cli.NewAccessiblePrompter(input, output).ConfirmReplacement(context.Background(), configPath)
				if err != nil {
					return err
				}
				if !confirmed {
					return nil
				}
			}
			return cli.Setup(context.Background(), input, output, configPath, defaultUser, setupDependencies(configPath))
		},
		start: func(output io.Writer, configPath string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			return cli.StartHeadless(ctx, output, configPath, startDependencies)
		},
		dashboard: func(input io.Reader, output io.Writer) error {
			if cli.UseTerminalApplication(input, output, deps.Getenv) {
				path, err := defaultConfigPath()
				if err != nil {
					return err
				}
				return cli.RunApplication(context.Background(), input, output, path, "root", "start", cli.ApplicationDependencies{Setup: setupDependencies(path), Start: startDependencies})
			}
			_, err := fmt.Fprintln(output, "The endpoint dashboard requires an interactive terminal. Run sovkit start to keep the tunnel open, then connect your application explicitly in another terminal.")
			return err
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
