package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/state"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

// refreshHostTrust re-pins known hosts when the SSH endpoint moved. A
// changed key is never silently accepted. Overridable in tests.
var refreshHostTrust = setup.RefreshSavedHostTrust

func runResume(args []string, input io.Reader, output io.Writer, configPath string) error {
	set := flag.NewFlagSet("resume", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	positionals, err := parseFlagsAroundPositional(set, args)
	if err != nil || len(positionals) > 1 {
		return usageErrorf("usage: sovkit resume [id]")
	}
	id := ""
	if len(positionals) == 1 {
		id = positionals[0]
	}
	dir, store, deployment, err := resolveTarget(configPath, id)
	if err != nil {
		return err
	}
	switch deployment.State {
	case state.Tunneled:
		return fmt.Errorf("deployment %q is already tunneled", deployment.ID)
	case state.Destroyed:
		return fmt.Errorf("deployment %q is already destroyed", deployment.ID)
	case state.Planned, state.Renting:
		return fmt.Errorf("nothing to resume for %q in state %s (provision with sovkit up)", deployment.ID, deployment.State)
	}
	if active, ok := store.ActiveDeployment(); ok && active.ID != deployment.ID && active.State.Live() {
		return fmt.Errorf("deployment %q owns the live route; down or destroy it first", active.ID)
	}
	token, err := state.Token(dir)
	if err != nil {
		return err
	}
	client := vast.NewClient(vastAPIBaseURL, token)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	instance, err := client.GetInstance(ctx, deployment.Instance.ID)
	if err != nil {
		return fmt.Errorf("read instance %d: %w", deployment.Instance.ID, err)
	}
	if !strings.EqualFold(instance.Status, "running") {
		fmt.Fprintf(output, "Instance %d is %s, requesting start and waiting…\n", deployment.Instance.ID, instance.Status)
		if err := client.StartInstance(ctx, deployment.Instance.ID); err != nil {
			var queued *vast.StateChangeError
			if !errors.As(err, &queued) || !queued.Queued() {
				return err
			}
		}
		if instance, err = waitInstanceRunning(ctx, client, deployment.Instance.ID, 5*time.Minute); err != nil {
			return err
		}
	}
	knownHosts := state.KnownHostsPath(dir, deployment.ID)
	oldSSH := config.SSH{KnownHostsFile: deployment.SSH.KnownHostsFile}
	if oldSSH.KnownHostsFile == "" {
		oldSSH.KnownHostsFile = knownHosts
	}
	nextSSH := config.SSH{
		Host: instance.SSHHost, Port: instance.SSHPort, User: deployment.SSH.User,
		IdentityFile: deployment.SSH.IdentityFile, KnownHostsFile: knownHosts,
	}
	if err := refreshHostTrust(ctx, oldSSH, nextSSH); err != nil {
		return err
	}
	deployment.SSH.Host, deployment.SSH.Port = instance.SSHHost, instance.SSHPort
	deployment.SSH.KnownHostsFile = knownHosts
	deployment.Instance.Status = instance.Status
	if err := persistDeployment(dir, &store, deployment); err != nil {
		return err
	}
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serveTunnel(sigCtx, output, dir, &store, deployment, client)
}

// waitInstanceRunning polls until the instance runs. Exited instances fail
// fast: restarting a broken container needs an operator decision.
func waitInstanceRunning(ctx context.Context, client *vast.Client, id int, timeout time.Duration) (vast.Instance, error) {
	deadline := time.Now().Add(timeout)
	for {
		instance, err := client.GetInstance(ctx, id)
		if err != nil {
			return vast.Instance{}, err
		}
		if strings.EqualFold(instance.Status, "running") {
			return instance, nil
		}
		if strings.EqualFold(instance.Status, "exited") {
			return instance, fmt.Errorf("instance %d exited; destroy and re-provision it", id)
		}
		if time.Now().After(deadline) {
			return instance, fmt.Errorf("instance %d not running after %s (status %q)", id, timeout, instance.Status)
		}
		select {
		case <-ctx.Done():
			return vast.Instance{}, ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}
