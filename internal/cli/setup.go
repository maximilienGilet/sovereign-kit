package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/maximilienGilet/sovereign-kit/internal/catalog"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/huggingface"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
)

type ManualRoute struct {
	Host           string
	Port           int
	User           string
	IdentityFile   string
	KnownHostsFile string
}

type CustomHardware struct {
	MinimumVRAMGB int
	MinimumDiskGB int
}

type VastWorkload struct {
	Recipe           recipe.Recipe
	AutoSelectOffer  bool
	Resume           bool
	ResumeInstanceID int
	RecoveryOnly     bool
}

type WorkloadPrompter interface {
	SelectWorkload(context.Context, []recipe.Recipe) (string, error)
	HuggingFaceQuery(context.Context) (string, error)
	SelectHuggingFaceModel(context.Context, []huggingface.SearchResult) (string, error)
	CustomHardware(context.Context) (CustomHardware, error)
	ConfirmCustomWorkload(context.Context, huggingface.Model, CustomHardware) (bool, error)
}

func resolveVastWorkload(
	ctx context.Context,
	prompter WorkloadPrompter,
	recipes []recipe.Recipe,
	search func(context.Context, string, int) ([]huggingface.SearchResult, error),
	inspect func(context.Context, string) (huggingface.Model, error),
) (VastWorkload, error) {
	if prompter == nil || len(recipes) == 0 {
		return VastWorkload{}, fmt.Errorf("Vast workload choices are required")
	}
	selected, err := prompter.SelectWorkload(ctx, recipes)
	if err != nil {
		return VastWorkload{}, err
	}
	selected = strings.TrimSpace(selected)
	if selected != customHuggingFaceWorkload {
		for _, candidate := range recipes {
			if candidate.ID == selected {
				if err := candidate.Validate(); err != nil {
					return VastWorkload{}, err
				}
				return VastWorkload{Recipe: candidate}, nil
			}
		}
		return VastWorkload{}, fmt.Errorf("unknown workload %q", selected)
	}
	query, err := prompter.HuggingFaceQuery(ctx)
	if err != nil {
		return VastWorkload{}, err
	}
	query = strings.TrimSpace(query)
	repository := query
	if strings.Count(query, "/") != 1 || strings.ContainsAny(query, " \t\n") {
		if search == nil {
			return VastWorkload{}, fmt.Errorf("Hugging Face model search service is required")
		}
		if observer, ok := prompter.(setup.ProgressObserver); ok {
			observer.SetupProgress(setup.Progress{Stage: setup.ProgressModelSearch})
		}
		results, err := search(ctx, query, 10)
		if err != nil {
			return VastWorkload{}, err
		}
		repository, err = prompter.SelectHuggingFaceModel(ctx, results)
		if err != nil {
			return VastWorkload{}, err
		}
		found := false
		for _, result := range results {
			if result.Repository == repository {
				found = true
				break
			}
		}
		if !found {
			return VastWorkload{}, fmt.Errorf("selected Hugging Face model is unavailable")
		}
	}
	if inspect == nil {
		return VastWorkload{}, fmt.Errorf("Hugging Face model inspection service is required")
	}
	if observer, ok := prompter.(setup.ProgressObserver); ok {
		observer.SetupProgress(setup.Progress{Stage: setup.ProgressModelInspect})
	}
	model, err := inspect(ctx, repository)
	if err != nil {
		return VastWorkload{}, err
	}
	if model.Classification.Status != catalog.Supported || model.Classification.Kind != "text-generation" || model.Classification.Engine != "sglang" {
		return VastWorkload{}, fmt.Errorf("Hugging Face model %s requires a reviewed recipe: %s", model.Repository, model.Classification.Reason)
	}
	hardware, err := prompter.CustomHardware(ctx)
	if err != nil {
		return VastWorkload{}, err
	}
	if hardware.MinimumVRAMGB < 1 || hardware.MinimumDiskGB < 1 {
		return VastWorkload{}, fmt.Errorf("custom model VRAM and disk requirements must be positive")
	}
	custom, err := recipe.CustomHuggingFace(model.Repository, model.Revision, false)
	if err != nil {
		return VastWorkload{}, err
	}
	custom.Requirements = recipe.Requirements{
		MinimumVRAMGB: hardware.MinimumVRAMGB,
		MinimumDiskGB: hardware.MinimumDiskGB,
	}
	if err := custom.Validate(); err != nil {
		return VastWorkload{}, err
	}
	confirmed, err := prompter.ConfirmCustomWorkload(ctx, model, hardware)
	if err != nil {
		return VastWorkload{}, err
	}
	if !confirmed {
		return VastWorkload{}, fmt.Errorf("Vast setup cancelled: custom workload was not confirmed")
	}
	return VastWorkload{Recipe: custom}, nil
}

type SetupPrompter interface {
	SelectProvider(context.Context) (string, error)
	VastAPIKey(context.Context) (string, error)
	ManualRoute(context.Context, string) (ManualRoute, error)
	VastIdentity(context.Context, string) (string, error)
}

type SetupDependencies struct {
	Credentials  VastCredentials
	Prompter     SetupPrompter
	Getenv       func(string) string
	HomeDir      func() (string, error)
	Recipes      []recipe.Recipe
	LoadRecipes  func() ([]recipe.Recipe, error)
	SearchModels func(context.Context, string, int) ([]huggingface.SearchResult, error)
	InspectModel func(context.Context, string) (huggingface.Model, error)
	RunVast      func(context.Context, string, string, VastWorkload, setup.Operator) (setup.Result, error)
}

func Setup(ctx context.Context, input io.Reader, output io.Writer, configPath, defaultUser string, deps SetupDependencies) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if deps.Getenv == nil {
		deps.Getenv = os.Getenv
	}
	if input == nil {
		input = os.Stdin
	}
	if output == nil {
		output = os.Stdout
	}
	if deps.Prompter == nil {
		if UseTerminalApplication(input, output, deps.Getenv) {
			deps.Prompter = NewHuhPrompter(input, output)
		} else {
			deps.Prompter = NewAccessiblePrompter(input, output)
		}
	}
	if deps.HomeDir == nil {
		deps.HomeDir = os.UserHomeDir
	}
	if accessible, ok := deps.Prompter.(*AccessiblePrompter); ok {
		accessible.recovery = setup.InstanceRecovery{}
	}
	if _, err := setup.ReadCheckpoint(setup.CheckpointPath(configPath)); !os.IsNotExist(err) {
		if err != nil {
			return fmt.Errorf("cannot read pending deployment: %w", err)
		}
		return ResumeSetup(ctx, output, configPath, deps, 0, false)
	}
	fmt.Fprintln(output, "Sovereign Kit setup")
	provider, err := deps.Prompter.SelectProvider(ctx)
	if err != nil {
		return err
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
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
	availableRecipes := deps.Recipes
	if len(availableRecipes) == 0 && deps.LoadRecipes != nil {
		var err error
		availableRecipes, err = deps.LoadRecipes()
		if err != nil {
			return fmt.Errorf("load built-in recipes: %w", err)
		}
	}
	workloadPrompter, ok := deps.Prompter.(WorkloadPrompter)
	if !ok {
		return fmt.Errorf("Vast setup prompter must implement workload selection")
	}
	workload, err := resolveVastWorkload(ctx, workloadPrompter, availableRecipes, deps.SearchModels, deps.InspectModel)
	if err != nil {
		return err
	}
	token, err := vastAPIKey(ctx, deps)
	if err != nil {
		return err
	}
	if deps.RunVast == nil {
		return fmt.Errorf("Vast runner is required")
	}
	home, err := deps.HomeDir()
	if err != nil {
		return fmt.Errorf("find home directory: %w", err)
	}
	identity, err := deps.Prompter.VastIdentity(ctx, filepath.Join(home, ".ssh", "sovkit_vast_ed25519"))
	if err != nil {
		return err
	}
	operator, ok := deps.Prompter.(setup.Operator)
	if !ok {
		return fmt.Errorf("Vast setup prompter must implement setup operator")
	}
	result, err := deps.RunVast(ctx, token, identity, workload, operator)
	if err != nil {
		if accessible, ok := deps.Prompter.(*AccessiblePrompter); ok {
			return accessible.recoverSetup(ctx, err, token)
		}
		return &redactedSetupError{cause: err, token: token}
	}
	fmt.Fprintf(output, "Vast instance %d created.\n", result.InstanceID)
	fmt.Fprintf(output, "Configuration saved: %s\n", result.ConfigPath)
	fmt.Fprintln(output, "Warning: billing may still be active for this instance.")
	fmt.Fprintln(output, "Next: sovkit start")
	return nil
}

// Keep error identity for callers without exposing the active API key in a
// provider's diagnostic when the legacy command prints the returned error.
type redactedSetupError struct {
	cause error
	token string
}

func (err *redactedSetupError) Error() string {
	if err.token == "" {
		return err.cause.Error()
	}
	return strings.ReplaceAll(err.cause.Error(), err.token, "[redacted]")
}
func (err *redactedSetupError) Unwrap() error { return err.cause }
