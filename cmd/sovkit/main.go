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
	"text/tabwriter"
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
		err = runWith(os.Args[1:], os.Stdin, os.Stdout, path)
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
	return runWith(args, os.Stdin, output, path)
}

func runWith(args []string, input io.Reader, output io.Writer, configPath string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(output, `Usage: sovkit <command>

Commands:
  recipes     List the embedded recipes
  offers      Search eligible Vast offers for a recipe (no renting)
  up          Provision a recipe: rent, prepare, serve, tunnel
  status      Show deployments (active in detail, others listed)
  down        Stop a deployment instance (billing paused)
  destroy     Destroy a deployment instance and its keys
  resume      Re-attach the tunnel to a deployment
  logs        Show remote server logs for a deployment
  doctor      Check the local route health`)
		return err
	}
	switch args[0] {
	case "recipes":
		return runRecipes(args[1:], input, output)
	case "offers":
		return runOffers(args[1:], input, output, configPath)
	case "up":
		return runUp(args[1:], input, output, configPath)
	case "status":
		return runStatus(args[1:], input, output, configPath)
	case "down":
		return runDown(args[1:], input, output, configPath)
	case "destroy":
		return runDestroy(args[1:], input, output, configPath)
	case "resume":
		return runResume(args[1:], input, output, configPath)
	case "logs":
		return runLogs(args[1:], input, output, configPath)
	case "doctor":
		return runDoctor(input, output, configPath)
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

func runRecipes(args []string, input io.Reader, output io.Writer) error {
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

// offerQuery is a validated offer search (no network yet).
type offerQuery struct {
	Recipe        recipe.Recipe
	Countries     []string
	GPUModel      string
	StrictGPU     bool
	Interruptible bool
	CapUSD        float64
	Limit         int
}

// resolveOfferQuery validates recipe + flags into a runnable query.
func resolveOfferQuery(resolved recipe.Recipe, store state.Store, gpuFlag, regionFlag string, countryFlags []string, interruptibleFlag bool, capFlag float64, limit int) (offerQuery, error) {
	model, strict, err := resolveGPUModel(resolved, gpuFlag)
	if err != nil {
		return offerQuery{}, err
	}
	bid, err := resolveInterruptible(resolved, interruptibleFlag)
	if err != nil {
		return offerQuery{}, err
	}
	codes, err := vast.GeographicCountries(regionFlag, countryFlags)
	if err != nil {
		return offerQuery{}, usageErrorf("invalid geography: %v", err)
	}
	cap := capFlag
	if cap <= 0 {
		cap = store.Settings.SpendCapUSD
	}
	return offerQuery{
		Recipe: resolved, Countries: codes, GPUModel: model, StrictGPU: strict,
		Interruptible: bid, CapUSD: cap, Limit: limit,
	}, nil
}

// runOfferQuery executes the search: offers, cap filter, rank at 730h.
func runOfferQuery(ctx context.Context, token string, query offerQuery) ([]planner.Recommendation, error) {
	offers, err := vast.NewClient(vastAPIBaseURL, token).SearchOffers(ctx, vast.SearchRequest{
		Countries: query.Countries, Sort: vast.SortPrice, Limit: query.Limit,
		GPUModel: query.GPUModel, GPUCount: query.Recipe.Requirements.GPUCount,
		StrictGPU:     query.StrictGPU,
		MinimumVRAMGB: query.Recipe.Requirements.MinimumVRAMGB,
		MinimumDiskGB: query.Recipe.Requirements.MinimumDiskGB,
		Interruptible: query.Interruptible,
	})
	if err != nil {
		return nil, err
	}
	return planner.Recommend(query.Recipe, filterOffersByCap(offers, query.CapUSD), monthlyHours), nil
}

// printOfferTable renders ranked recommendations for choosing, columns
// aligned even when cells contain spaces (GPU names, locations).
func printOfferTable(output io.Writer, recommendations []planner.Recommendation) {
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "#\tID\t$/H\t$/MO\tGPU\tLOCATION\tDOWN/UP\tREL\tDRIVER")
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
		fmt.Fprintf(writer, "%d\t%d\t%s\t%s\t%d× %s\t%s\t%.0f/%.0f\t%s\t%s\n",
			index+1, offer.ID, price, monthly, offer.GPUCount, offer.GPUName,
			offer.Location, offer.InetDownMBps, offer.InetUpMBps, reliability, offer.DriverVersion)
	}
	_ = writer.Flush()
}

func runOffers(args []string, input io.Reader, output io.Writer, configPath string) error {
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
	dir := filepath.Dir(configPath)
	store, err := state.Load(dir)
	if err != nil {
		return err
	}
	query, err := resolveOfferQuery(resolved, store, *gpu, *region, countries, *interruptible, *capUSD, *limit)
	if err != nil {
		return err
	}
	token, err := state.Token(dir)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	recommendations, err := runOfferQuery(ctx, token, query)
	if err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(output, recommendations)
	}
	if len(recommendations) == 0 {
		_, err := fmt.Fprintf(output, "No eligible offers for %s.\n", resolved.ID)
		return err
	}
	printOfferTable(output, recommendations)
	return nil
}

func runStatus(args []string, input io.Reader, output io.Writer, configPath string) error {
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
	if len(positionals) == 1 {
		deployment, err := resolveDeployment(dir, store, positionals[0])
		if err != nil {
			return err
		}
		return printDeployment(output, deployment, *asJSON)
	}
	active, ok := store.ActiveDeployment()
	if !ok {
		if len(store.Deployments) == 0 {
			return noActiveError(dir, store)
		}
		return printDeploymentList(output, store.Deployments, *asJSON)
	}
	if err := printDeployment(output, active, *asJSON); err != nil {
		return err
	}
	if *asJSON {
		return nil
	}
	var others []state.Deployment
	for _, deployment := range store.Deployments {
		if deployment.ID != active.ID {
			others = append(others, deployment)
		}
	}
	if len(others) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(output, "Other deployments:"); err != nil {
		return err
	}
	return printDeploymentList(output, others, false)
}

func printDeployment(output io.Writer, deployment state.Deployment, asJSON bool) error {
	if asJSON {
		return writeJSON(output, deployment)
	}
	cap := "none"
	if deployment.CapUSD > 0 {
		cap = fmt.Sprintf("$%.4g/h", deployment.CapUSD)
	}
	_, err := fmt.Fprintf(output, `%s · %s
  recipe %s · instance %d (%s)
  route http://%s:%d
  spend $%.4g/h · $%.2f total · cap %s
`,
		deployment.ID, deployment.State, deployment.RecipeID,
		deployment.Instance.ID, deployment.Instance.Status,
		deployment.Route.LocalHost, deployment.Route.LocalPort,
		deployment.Spend.HourlyUSD, deployment.Spend.TotalUSD, cap)
	return err
}

// printDeploymentList shows one line per deployment, newest first. Stopped
// and destroyed records stay visible so their ids remain discoverable.
func printDeploymentList(output io.Writer, deployments []state.Deployment, asJSON bool) error {
	if asJSON {
		return writeJSON(output, deployments)
	}
	for index := len(deployments) - 1; index >= 0; index-- {
		deployment := deployments[index]
		if _, err := fmt.Fprintf(output, "  %s · %s · %s · instance %d (%s) · :%d · $%.4g/h\n",
			deployment.ID, deployment.State, deployment.RecipeID,
			deployment.Instance.ID, deployment.Instance.Status,
			deployment.Route.LocalPort, deployment.Spend.HourlyUSD); err != nil {
			return err
		}
	}
	return nil
}

func runDoctor(input io.Reader, output io.Writer, configPath string) error {
	if handled, err := runInstalledDoctor(output); handled {
		return err
	}
	_, _, deployment, err := resolveTarget(configPath, "")
	if err != nil {
		return err
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
