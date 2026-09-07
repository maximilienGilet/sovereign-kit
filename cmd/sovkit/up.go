package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/planner"
	"github.com/maximilienGilet/sovereign-kit/internal/provision"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/state"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

// provisionPrepare runs the rent-to-serve flow. Overridable in tests.
var provisionPrepare = provision.Provision

// autoConfirmIdentity assents to local SSH identity setup. Spending is
// gated separately by the rent choice prompt, so key setup needs none.
type autoConfirmIdentity struct{}

func (autoConfirmIdentity) ConfirmIdentitySetup(context.Context, string, bool) (bool, error) {
	return true, nil
}

func defaultProvisionDeps(client *vast.Client, input io.Reader, output io.Writer, progress func(string)) provision.Deps {
	return provision.Deps{
		Vast: client,
		Identity: provision.IdentityFunc(func(ctx context.Context, path string) error {
			return setup.PrepareVastIdentity(ctx, path, client, setup.ExecRunner{}, autoConfirmIdentity{})
		}),
		Scanner:  setup.SystemHostKeyScanner{Runner: setup.ExecRunner{}},
		Trust:    setup.FileTrustStore{},
		Launcher: setup.StrictSSHLauncher{Runner: setup.ExecRunner{}, Clock: setup.RealClock{}},
		Clock:    setup.RealClock{},
		Confirm: func(_ context.Context, fingerprints []string) (bool, error) {
			fmt.Fprintln(output, "Remote host key fingerprints:")
			for _, fingerprint := range fingerprints {
				fmt.Fprintln(output, "  "+fingerprint)
			}
			fmt.Fprint(output, "Trust these keys and continue? [y/N]: ")
			answer := readConfirmLine(input)
			return answer == "y" || answer == "yes", nil
		},
		Progress: progress,
	}
}

// findOffer selects one eligible offer by id.
func findOffer(recommendations []planner.Recommendation, id int) (planner.Recommendation, error) {
	for _, recommendation := range recommendations {
		if recommendation.Offer.ID == id {
			return recommendation, nil
		}
	}
	return planner.Recommendation{}, fmt.Errorf("offer %d is not eligible (see sovkit offers)", id)
}

// chooseOffer applies the selection policy: explicit --offer wins, --yes
// takes the cheapest, interactive terminals get one choice prompt that is
// also the purchase confirmation, and non-interactive runs without --yes
// refuse when several offers match.
func chooseOffer(input io.Reader, output io.Writer, recommendations []planner.Recommendation, offerFlag int, useYes bool) (planner.Recommendation, error) {
	if offerFlag > 0 {
		return findOffer(recommendations, offerFlag)
	}
	if useYes {
		return recommendations[0], nil
	}
	if !stdinInteractive(input) {
		if len(recommendations) == 1 {
			return recommendations[0], nil
		}
		return planner.Recommendation{}, fmt.Errorf("multiple offers match; refine with --country/--region, pick --offer, or confirm with --yes")
	}
	printOfferTable(output, recommendations)
	for attempt := 0; attempt < 3; attempt++ {
		fmt.Fprintf(output, "Select offer [1] (Enter = cheapest, $%.4g/h): ", recommendations[0].Offer.HourlyUSD)
		line := readConfirmLine(input)
		if line == "" {
			return recommendations[0], nil
		}
		choice, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || choice < 1 || choice > len(recommendations) {
			fmt.Fprintf(output, "Enter 1-%d or empty for cheapest.\n", len(recommendations))
			continue
		}
		return recommendations[choice-1], nil
	}
	return planner.Recommendation{}, fmt.Errorf("no valid selection")
}

func runUp(args []string, input io.Reader, output io.Writer, configPath string) error {
	var countries countryList
	set := flag.NewFlagSet("up", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	offerFlag := set.Int("offer", 0, "rent a specific eligible offer id")
	dryRun := set.Bool("dry-run", false, "preview without renting")
	useYes := set.Bool("yes", false, "rent the cheapest eligible offer without asking")
	capUSD := set.Float64("cap", 0, "max hourly USD (0 = state default)")
	region := set.String("region", "", "geographic region filter (e.g. europe)")
	gpu := set.String("gpu", "", "GPU model filter (narrows only)")
	interruptible := set.Bool("interruptible", false, "include interruptible (bid) offers")
	verifyHostKey := set.Bool("verify-host-key", false, "block on manual host-key approval at first contact")
	set.Var(&countries, "country", "country code filter, repeatable (e.g. FR)")
	positionals, err := parseFlagsAroundPositional(set, args)
	if err != nil || len(positionals) != 1 {
		return usageErrorf("usage: sovkit up <recipe> [--offer ID] [--dry-run] [--yes] [--cap USD] [--country CC] [--region R] [--gpu MODEL] [--interruptible] [--verify-host-key]")
	}
	if *verifyHostKey && *useYes {
		return usageErrorf("--verify-host-key needs an interactive terminal without --yes")
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
	query, err := resolveOfferQuery(resolved, store, *gpu, *region, countries, *interruptible, *capUSD, 20)
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
	if len(recommendations) == 0 {
		_, err := fmt.Fprintf(output, "No eligible offers for %s.\n", resolved.ID)
		return err
	}
	offerType := "ondemand"
	if query.Interruptible {
		offerType = "bid (interruptible)"
	}
	if *dryRun {
		preview := recommendations[0]
		if *offerFlag > 0 {
			var err error
			preview, err = findOffer(recommendations, *offerFlag)
			if err != nil {
				return err
			}
		}
		fmt.Fprintln(output, "Dry run — nothing will be rented, no state changed.")
		fmt.Fprintf(output, "Recipe: %s (%s)\n", resolved.ID, resolved.Name)
		printOfferTable(output, recommendations)
		fmt.Fprintf(output, "Would rent: offer #%d · %d× %s · %s · $%.4g/h ($%.0f/mo) · type %s · cap %s\n",
			preview.Offer.ID, preview.Offer.GPUCount, preview.Offer.GPUName, preview.Offer.Location,
			preview.Offer.HourlyUSD, preview.MonthlyUSD, offerType, capLabel(query.CapUSD))
		return nil
	}
	chosen, err := chooseOffer(input, output, recommendations, *offerFlag, *useYes)
	if err != nil {
		return err
	}
	if *verifyHostKey && !stdinInteractive(input) {
		return usageErrorf("--verify-host-key needs an interactive terminal without --yes")
	}
	if err := requireNoLiveDeployment(store); err != nil {
		return err
	}
	port, err := store.AllocatePort()
	if err != nil {
		return err
	}
	client := vast.NewClient(vastAPIBaseURL, token)
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	spin := newSpinner(os.Stderr, "Provisioning "+resolved.ID)
	deps := defaultProvisionDeps(client, input, output, func(line string) {
		fmt.Fprintln(os.Stderr, line)
		spin.SetMessage(line)
	})
	deployment, err := provisionPrepare(sigCtx, &store, dir, provision.Inputs{
		Recipe: resolved, Offer: chosen.Offer, CapUSD: query.CapUSD,
		Port: port, DeploymentID: store.NextID(resolved.ID, time.Now()),
		VerifyHostKey: *verifyHostKey, ReadyTimeout: 10 * time.Minute, ScanTimeout: 3 * time.Minute,
	}, deps)
	if err != nil {
		spin.Stop("Provisioning failed")
		return err
	}
	spin.Stop("Server launched")
	return serveTunnel(sigCtx, output, dir, &store, deployment, client)
}

func capLabel(maxHourly float64) string {
	if maxHourly <= 0 {
		return "none"
	}
	return fmt.Sprintf("$%.4g/h", maxHourly)
}
