package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/planner"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/route"
	"github.com/maximilienGilet/sovereign-kit/internal/state"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
	"github.com/maximilienGilet/sovereign-kit/recipes"
)

// vastAPIBaseURL is a variable so tests can point the CLI at a fake server.
var vastAPIBaseURL = "https://console.vast.ai"

// monthlyHours is the calendar-month base for cost projections.
const monthlyHours = 730

type usageError struct{ msg string }

func (err *usageError) Error() string { return err.msg }

func usageErrorf(format string, args ...any) *usageError {
	return &usageError{msg: fmt.Sprintf(format, args...)}
}

type doctorExitError struct {
	code int
}

func (err *doctorExitError) Error() string {
	return fmt.Sprintf("doctor exited with status %d", err.code)
}

// exitCode maps errors to the CLI contract: 0 success, 1 operational
// failure, 2 usage error. The installed doctor helper keeps its own code.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var doctorExit *doctorExitError
	if errors.As(err, &doctorExit) {
		return doctorExit.code
	}
	var usage *usageError
	if errors.As(err, &usage) {
		return 2
	}
	return 1
}

func main() {
	path, err := defaultConfigPath()
	if err == nil {
		err = runWith(os.Args[1:], os.Stdout, path)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "sovkit:", err)
		os.Exit(exitCode(err))
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
  recipes     List the embedded recipes
  offers      Search eligible Vast offers for a recipe (no renting)
  status      Show a deployment (the active one by default)
  doctor      Check the local route health`)
		return err
	}
	switch args[0] {
	case "recipes":
		return runRecipes(args[1:], output)
	case "offers":
		return runOffers(args[1:], output, configPath)
	case "status":
		return runStatus(args[1:], output, configPath)
	case "doctor":
		return runDoctor(output, configPath)
	default:
		return usageErrorf("unknown command %q (try: sovkit help)", args[0])
	}
}

// parseFlagsAroundPositional parses flags that may appear before or after positional args.
// The standard flag package stops at the first positional, so parse twice:
// once for leading flags, then again for the remainder. Values starting
// with a dash are not supported; none of the flags take such values.
func parseFlagsAroundPositional(set *flag.FlagSet, args []string) ([]string, error) {
	if err := set.Parse(args); err != nil {
		return nil, err
	}
	rest := set.Args()
	if len(rest) == 0 {
		return nil, nil
	}
	positionals := []string{rest[0]}
	if err := set.Parse(rest[1:]); err != nil {
		return nil, err
	}
	return append(positionals, set.Args()...), nil
}

// countryList accepts repeated --country flags and comma-separated codes.
type countryList []string

func (list *countryList) String() string { return strings.Join(*list, ",") }

func (list *countryList) Set(value string) error {
	for _, item := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			*list = append(*list, trimmed)
		}
	}
	return nil
}

// writeJSON marshals one result value to stdout.
func writeJSON(output io.Writer, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, string(encoded))
	return err
}

func builtinRecipe(id string) (recipe.Recipe, error) {
	list, err := recipes.Builtin()
	if err != nil {
		return recipe.Recipe{}, err
	}
	for _, candidate := range list {
		if candidate.ID == id {
			return candidate, nil
		}
	}
	return recipe.Recipe{}, usageErrorf("unknown recipe %q (try: sovkit recipes)", id)
}

// resolveGPUModel applies the recipe floor: flags narrow only. A --gpu flag
// against a strict recipe must name the same model or it is refused.
func resolveGPUModel(resolved recipe.Recipe, flag string) (model string, strict bool, err error) {
	if flag == "" {
		return resolved.Requirements.GPUModel, resolved.Requirements.StrictGPU, nil
	}
	if resolved.Requirements.StrictGPU && flag != resolved.Requirements.GPUModel {
		return "", false, usageErrorf("recipe %q requires exactly %q (strict); cannot search %q", resolved.ID, resolved.Requirements.GPUModel, flag)
	}
	return flag, true, nil
}

// resolveInterruptible enforces the double gate: the flag alone never pulls
// interruptible offers for a recipe that forbids them.
func resolveInterruptible(resolved recipe.Recipe, flag bool) (bool, error) {
	if flag && !resolved.Requirements.AllowInterruptible {
		return false, usageErrorf("recipe %q forbids interruptible offers", resolved.ID)
	}
	return flag, nil
}

// filterOffersByCap drops offers above the hourly cap. Unknown prices fail
// closed: an unverifiable price cannot prove it fits the cap.
func filterOffersByCap(offers []vast.Offer, maxHourly float64) []vast.Offer {
	if maxHourly <= 0 {
		return offers
	}
	kept := make([]vast.Offer, 0, len(offers))
	for _, offer := range offers {
		if offer.PriceUnknown || offer.HourlyUSD > maxHourly {
			continue
		}
		kept = append(kept, offer)
	}
	return kept
}

// legacyHint notes the superseded single-route config when the store is
// empty, so old setups learn the way forward.
func legacyHint(dir string, store state.Store) string {
	if len(store.Deployments) > 0 {
		return ""
	}
	if _, err := os.Stat(state.LegacyConfigPath(dir)); err == nil {
		return " Legacy config.toml found: re-provision with sovkit up."
	}
	return ""
}

func runRecipes(args []string, output io.Writer) error {
	set := flag.NewFlagSet("recipes", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	asJSON := set.Bool("json", false, "machine-readable output")
	if err := set.Parse(args); err != nil {
		return usageErrorf("usage: sovkit recipes [--json]")
	}
	if set.NArg() > 0 {
		return usageErrorf("usage: sovkit recipes [--json]")
	}
	list, err := recipes.Builtin()
	if err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(output, list)
	}
	for _, item := range list {
		fmt.Fprintf(output, "%s · %s\n  engine %s · %s · %s\n  %d× %s · ≥%dGB VRAM · ≥%dGB disk\n",
			item.ID, item.Name, item.Runtime.Engine, item.Kind, item.Profile.Status,
			item.Requirements.GPUCount, item.Requirements.GPUModel,
			item.Requirements.MinimumVRAMGB, item.Requirements.MinimumDiskGB)
	}
	return nil
}

func runOffers(args []string, output io.Writer, configPath string) error {
	var countries countryList
	set := flag.NewFlagSet("offers", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	asJSON := set.Bool("json", false, "machine-readable output")
	limit := set.Int("limit", 20, "max offers to list (1-100)")
	region := set.String("region", "", "geographic region filter (e.g. europe)")
	gpu := set.String("gpu", "", "GPU model filter (narrows only)")
	interruptible := set.Bool("interruptible", false, "include interruptible (bid) offers")
	capUSD := set.Float64("cap", 0, "max hourly USD (0 = state default)")
	set.Var(&countries, "country", "country code filter, repeatable (e.g. FR)")
	positionals, err := parseFlagsAroundPositional(set, args)
	if err != nil {
		return usageErrorf("usage: sovkit offers <recipe> [--json] [--limit N] [--country CC] [--region R] [--gpu MODEL] [--interruptible] [--cap USD]")
	}
	if len(positionals) != 1 {
		return usageErrorf("usage: sovkit offers <recipe> [--json] [--limit N] [--country CC] [--region R] [--gpu MODEL] [--interruptible] [--cap USD]")
	}
	resolved, err := builtinRecipe(positionals[0])
	if err != nil {
		return err
	}
	model, strict, err := resolveGPUModel(resolved, *gpu)
	if err != nil {
		return err
	}
	bid, err := resolveInterruptible(resolved, *interruptible)
	if err != nil {
		return err
	}
	dir := filepath.Dir(configPath)
	store, err := state.Load(dir)
	if err != nil {
		return err
	}
	cap := *capUSD
	if cap <= 0 {
		cap = store.Settings.SpendCapUSD
	}
	codes, err := vast.GeographicCountries(*region, countries)
	if err != nil {
		return usageErrorf("invalid geography: %v", err)
	}
	token, err := state.Token(dir)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	offers, err := vast.NewClient(vastAPIBaseURL, token).SearchOffers(ctx, vast.SearchRequest{
		Countries:     codes,
		Sort:          vast.SortPrice,
		Limit:         *limit,
		GPUModel:      model,
		GPUCount:      resolved.Requirements.GPUCount,
		StrictGPU:     strict,
		MinimumVRAMGB: resolved.Requirements.MinimumVRAMGB,
		MinimumDiskGB: resolved.Requirements.MinimumDiskGB,
		Interruptible: bid,
	})
	if err != nil {
		return err
	}
	recommendations := planner.Recommend(resolved, filterOffersByCap(offers, cap), monthlyHours)
	if *asJSON {
		return writeJSON(output, recommendations)
	}
	if len(recommendations) == 0 {
		_, err := fmt.Fprintf(output, "No eligible offers for %s.\n", resolved.ID)
		return err
	}
	fmt.Fprintln(output, "#  ID  $/H  $/MO  GPU  LOCATION  DOWN/UP  REL  DRIVER")
	for index, recommendation := range recommendations {
		offer := recommendation.Offer
		price, monthly := "unknown", "unknown"
		if !offer.PriceUnknown {
			price = fmt.Sprintf("%.4g", offer.HourlyUSD)
			monthly = fmt.Sprintf("%.0f", recommendation.MonthlyUSD)
		}
		reliability := "unknown"
		if !offer.ReliabilityUnknown {
			reliability = fmt.Sprintf("%.1f%%", offer.Reliability*100)
		}
		fmt.Fprintf(output, "%d  %d  %s  %s  %d× %s  %s  %.0f/%.0f  %s  %s\n",
			index+1, offer.ID, price, monthly, offer.GPUCount, offer.GPUName,
			offer.Location, offer.InetDownMBps, offer.InetUpMBps, reliability, offer.DriverVersion)
	}
	return nil
}

func runStatus(args []string, output io.Writer, configPath string) error {
	set := flag.NewFlagSet("status", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	asJSON := set.Bool("json", false, "machine-readable output")
	positionals, err := parseFlagsAroundPositional(set, args)
	if err != nil {
		return usageErrorf("usage: sovkit status [id] [--json]")
	}
	if len(positionals) > 1 {
		return usageErrorf("usage: sovkit status [id] [--json]")
	}
	dir := filepath.Dir(configPath)
	store, err := state.Load(dir)
	if err != nil {
		return err
	}
	var deployment state.Deployment
	if len(positionals) == 1 {
		found, ok := store.Get(positionals[0])
		if !ok {
			return fmt.Errorf("unknown deployment %q", positionals[0])
		}
		deployment = found
	} else {
		active, ok := store.ActiveDeployment()
		if !ok {
			return fmt.Errorf("no active deployment.%s", legacyHint(dir, store))
		}
		deployment = active
	}
	if *asJSON {
		return writeJSON(output, deployment)
	}
	cap := "none"
	if deployment.CapUSD > 0 {
		cap = fmt.Sprintf("$%.4g/h", deployment.CapUSD)
	}
	fmt.Fprintf(output, `%s · %s
  recipe %s · instance %d (%s)
  route http://%s:%d
  spend $%.4g/h · $%.2f total · cap %s
`,
		deployment.ID, deployment.State, deployment.RecipeID,
		deployment.Instance.ID, deployment.Instance.Status,
		deployment.Route.LocalHost, deployment.Route.LocalPort,
		deployment.Spend.HourlyUSD, deployment.Spend.TotalUSD, cap)
	return nil
}

func runDoctor(output io.Writer, configPath string) error {
	if handled, err := runInstalledDoctor(output); handled {
		return err
	}
	dir := filepath.Dir(configPath)
	store, err := state.Load(dir)
	if err != nil {
		return err
	}
	deployment, ok := store.ActiveDeployment()
	if !ok {
		return fmt.Errorf("no active deployment.%s", legacyHint(dir, store))
	}
	endpoint := fmt.Sprintf("http://%s:%d", deployment.Route.LocalHost, deployment.Route.LocalPort)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := route.Healthcheck(ctx, endpoint); err != nil {
		return fmt.Errorf("local route %s is unavailable: %w", endpoint, err)
	}
	_, err = fmt.Fprintf(output, "PASS  local route answered %s/v1/models\n", endpoint)
	return err
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
