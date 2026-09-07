package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/state"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

func runDown(args []string, input io.Reader, output io.Writer, configPath string) error {
	set := flag.NewFlagSet("down", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	positionals, err := parseFlagsAroundPositional(set, args)
	if err != nil || len(positionals) > 1 {
		return usageErrorf("usage: sovkit down [id]")
	}
	dir := filepath.Dir(configPath)
	store, err := state.Load(dir)
	if err != nil {
		return err
	}
	id := ""
	if len(positionals) == 1 {
		id = positionals[0]
	}
	deployment, err := resolveDeployment(dir, store, id)
	if err != nil {
		return err
	}
	switch deployment.State {
	case state.Destroyed:
		return fmt.Errorf("deployment %q is already destroyed", deployment.ID)
	case state.Stopped:
		_, err := fmt.Fprintf(output, "Deployment %q is already stopped.\n", deployment.ID)
		return err
	}
	if deployment.Instance.ID <= 0 {
		return fmt.Errorf("deployment %q has no instance to stop", deployment.ID)
	}
	token, err := state.Token(dir)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := vast.NewClient(vastAPIBaseURL, token).StopInstance(ctx, deployment.Instance.ID); err != nil {
		return err
	}
	deployment.State = state.Stopped
	if store.Active == deployment.ID {
		store.ClearActive()
	}
	if err := persistDeployment(dir, &store, deployment); err != nil {
		return err
	}
	_, err = fmt.Fprintf(output, "Stopped instance %d (billing paused).\nResume with `sovkit resume %s` or end billing with `sovkit destroy %s`.\n", deployment.Instance.ID, deployment.ID, deployment.ID)
	return err
}
