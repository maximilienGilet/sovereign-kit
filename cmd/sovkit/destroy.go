package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/sshkey"
	"github.com/maximilienGilet/sovereign-kit/internal/state"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

func runDestroy(args []string, input io.Reader, output io.Writer, configPath string) error {
	set := flag.NewFlagSet("destroy", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	confirmYes := set.Bool("yes", false, "skip confirmation (scripts)")
	positionals, err := parseFlagsAroundPositional(set, args)
	if err != nil || len(positionals) > 1 {
		return usageErrorf("usage: sovkit destroy [id] [--yes]")
	}
	id := ""
	if len(positionals) == 1 {
		id = positionals[0]
	}
	dir, store, deployment, err := resolveTarget(configPath, id)
	if err != nil {
		return err
	}
	if deployment.State == state.Destroyed {
		return fmt.Errorf("deployment %q is already destroyed", deployment.ID)
	}
	age := "unknown age"
	if !deployment.CreatedAt.IsZero() {
		age = time.Since(deployment.CreatedAt).Round(time.Minute).String() + " ago"
	}
	fmt.Fprintf(output, "Deployment %s · instance %d\n  rate $%.4g/h · recorded total $%.2f · created %s\n  Exact cumulative billing: Vast console.\n",
		deployment.ID, deployment.Instance.ID, deployment.Spend.HourlyUSD, deployment.Spend.TotalUSD, age)
	if !*confirmYes {
		if !stdinInteractive(input) {
			return fmt.Errorf("destroy requires confirmation (use --yes in scripts)")
		}
		fmt.Fprintf(output, "Destroy instance %d and delete its SSH keys? [y/N]: ", deployment.Instance.ID)
		if answer := readConfirmLine(input); answer != "y" && answer != "yes" {
			_, err := fmt.Fprintln(output, "Cancelled.")
			return err
		}
	}
	token, err := state.Token(dir)
	if err != nil {
		return err
	}
	client := vast.NewClient(vastAPIBaseURL, token)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if deployment.Instance.ID > 0 {
		exists, err := client.InstanceExists(ctx, deployment.Instance.ID)
		if err != nil {
			return err
		}
		if exists {
			if err := client.DestroyInstance(ctx, deployment.Instance.ID); err != nil {
				deployment.State = state.Failed
				if saveErr := persistDeployment(dir, &store, deployment); saveErr != nil {
					return fmt.Errorf("destroy failed: %v (and the failure could not be recorded: %v)", err, saveErr)
				}
				return fmt.Errorf("destroy failed; instance %d may still bill: %v", deployment.Instance.ID, err)
			}
		} else {
			fmt.Fprintf(output, "Instance %d is already gone remotely.\n", deployment.Instance.ID)
		}
	}
	if err := finalizeGoneDeployment(ctx, client, dir, &store, deployment, output); err != nil {
		return fmt.Errorf("destroyed, but cleanup failed (remove leftovers manually): %v", err)
	}
	wasTunneled := deployment.State == state.Tunneled || deployment.State == state.Serving
	if wasTunneled {
		fmt.Fprintln(output, "If sovkit up or resume is running for this deployment, stop it (Ctrl-C); this command only affects the remote instance.")
	}
	_, err = fmt.Fprintf(output, "Destroyed instance %d.\n", deployment.Instance.ID)
	return err
}

// finalizeGoneDeployment cleans local residue for an instance Vast no
// longer knows: account key, files, record. Every step is attempted;
// failures join into one error. Billing already ended remotely.
func finalizeGoneDeployment(ctx context.Context, client *vast.Client, dir string, store *state.Store, deployment state.Deployment, output io.Writer) error {
	removed, err := removeDeploymentKeys(ctx, client, dir, deployment)
	if err == nil && !removed {
		fmt.Fprintln(output, "SSH key already absent from the Vast account.")
	}
	filesErr := os.RemoveAll(state.DeploymentDir(dir, deployment.ID))
	deployment.State = state.Destroyed
	if store.Active == deployment.ID {
		store.ClearActive()
	}
	return errors.Join(err, filesErr, persistDeployment(dir, store, deployment))
}

// removeDeploymentKeys deletes the deployment public key from the Vast
// account. It reports whether a key was removed: absence is verifiably
// clean, while an unreadable local key or a failed delete is an error.
func removeDeploymentKeys(ctx context.Context, client *vast.Client, dir string, deployment state.Deployment) (bool, error) {
	public, err := os.ReadFile(state.IdentityPath(dir, deployment.ID) + ".pub")
	if err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf("identity file already gone, cannot verify key cleanup")
		}
		return false, err
	}
	normalized, err := sshkey.Normalize(strings.TrimSpace(string(public)))
	if err != nil {
		return false, fmt.Errorf("local public key is invalid: %w", err)
	}
	keys, err := client.ListSSHKeys(ctx)
	if err != nil {
		return false, err
	}
	for _, key := range keys {
		candidate, err := sshkey.Normalize(key.Key)
		if err != nil || candidate != normalized {
			continue
		}
		if err := client.DeleteSSHKey(ctx, key.ID); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}
