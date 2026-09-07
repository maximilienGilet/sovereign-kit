package cli

import (
	"context"
	"errors"
	"fmt"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
	"reflect"
	"strings"
	"time"
)

type reconnectAPI interface {
	GetInstance(context.Context, int) (vast.Instance, error)
	StartInstance(context.Context, int) error
}
type reconnectConfirmation func(context.Context, int) (bool, error)
type connectionPreparer func(context.Context, config.Config, reconnectConfirmation, func(string)) (config.Config, error)

func prepareReconnect(ctx context.Context, cfg config.Config, api reconnectAPI, confirm reconnectConfirmation, progress func(string), restore func(context.Context, config.Config) (config.Config, error), interval time.Duration) (config.Config, error) {
	id := cfg.Provider.InstanceID
	read := func() (vast.Instance, error) {
		callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		instance, err := api.GetInstance(callCtx, id)
		if err == nil && instance.ID != id {
			err = fmt.Errorf("Vast returned a different instance; refusing reconnect")
		}
		return instance, err
	}
	progress("Checking Vast instance status…")
	instance, err := read()
	if err != nil {
		return cfg, err
	}
	restarted := false
	queued := instance.Status != "running" && instance.StartRequested()
	if strings.ToLower(instance.Status) == "exited" && !queued {
		approved, err := confirm(ctx, id)
		if err != nil {
			return cfg, err
		}
		if !approved {
			return cfg, context.Canceled
		}
		if err := ctx.Err(); err != nil {
			return cfg, err
		}
		// Consent may have stayed open while another client changed the instance.
		instance, err = read()
		if err != nil {
			return cfg, err
		}
		queued = instance.StartRequested()
		if strings.ToLower(instance.Status) == "exited" && !queued {
			progress("Restarting Vast instance…")
			callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			err = api.StartInstance(callCtx, id)
			cancel()
			if err != nil {
				var stateErr *vast.StateChangeError
				if errors.As(err, &stateErr) && stateErr.Queued() {
					queued = true
				} else {
					return cfg, fmt.Errorf("restart request failed; check Vast before retrying: %w", err)
				}
			}
		}
		restarted = true
	}
	deadline := time.NewTimer(15 * time.Minute)
	defer deadline.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return cfg, err
		}
		state := strings.ToLower(instance.Status)
		if state == "running" && instance.SSHHost != "" && instance.SSHPort > 0 {
			break
		}
		switch state {
		case "created", "creating", "loading", "pending", "starting", "running":
		case "exited":
			if !restarted && !queued {
				return cfg, fmt.Errorf("instance is stopped")
			}
		default:
			return cfg, fmt.Errorf("Vast instance #%d is %q; no SSH connection attempted", id, instance.Status)
		}
		if queued && state == "exited" {
			progress("Waiting for Vast resources · request queued…")
		} else {
			progress(fmt.Sprintf("Waiting for Vast instance · %s…", state))
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return cfg, ctx.Err()
		case <-deadline.C:
			timer.Stop()
			return cfg, fmt.Errorf("stopped waiting for instance #%d after 15 minutes; the remote start request may remain queued and resume GPU billing later. Use Stop or Destroy to cancel it; saved configuration retained", id)
		case <-timer.C:
		}
		instance, err = read()
		if err != nil {
			return cfg, err
		}
	}
	cfg.SSH.Host, cfg.SSH.Port = instance.SSHHost, instance.SSHPort
	if restarted || cfg.RestartPending {
		progress("Restoring inference server…")
		return restore(ctx, cfg)
	}
	return cfg, nil
}

func (m *applicationModel) reconnectPreparer() connectionPreparer {
	if m.deps.Start.PrepareConnection != nil {
		return m.deps.Start.PrepareConnection
	}
	token := m.readAvailableVastToken()
	m.rememberSecret(token)
	path := m.path
	return func(ctx context.Context, cfg config.Config, confirm reconnectConfirmation, progress func(string)) (config.Config, error) {
		if cfg.Provider.Kind != "vast" {
			return cfg, nil
		}
		if token == "" {
			return cfg, fmt.Errorf("Vast API key required to check instance state before reconnecting")
		}
		original := cfg
		check := func() error {
			current, err := config.Load(path)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(current, original) {
				return fmt.Errorf("saved configuration changed; reconnect again")
			}
			return nil
		}
		guardedConfirm := func(ctx context.Context, id int) (bool, error) {
			ok, err := confirm(ctx, id)
			if err == nil && ok {
				err = check()
				if err == nil {
					pending := original
					pending.RestartPending = true
					err = config.Save(path, pending)
					if err == nil {
						original = pending
					}
				}
			}
			return ok, err
		}
		restored, err := prepareReconnect(ctx, cfg, vast.NewClient("https://console.vast.ai", token), guardedConfirm, progress, setup.RestartSavedServer, 5*time.Second)
		if err != nil {
			return cfg, err
		}
		if err := check(); err != nil {
			return cfg, err
		}
		if restored.SSH != original.SSH {
			if err := setup.RefreshSavedHostTrust(ctx, original.SSH, restored.SSH); err != nil {
				return cfg, err
			}
		}
		if err := ctx.Err(); err != nil {
			return cfg, err
		}
		restored.RestartPending = false
		if err := config.Save(path, restored); err != nil {
			return cfg, err
		}
		return restored, nil
	}
}
