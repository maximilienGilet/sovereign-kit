package setup

import (
	"context"
	"fmt"
	"strings"

	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

// InstanceStopper keeps stop verification separate from provisioning's API.
type InstanceStopper interface {
	StopInstance(context.Context, int) error
	GetInstance(context.Context, int) (vast.Instance, error)
}

func stopAndVerifyInstance(ctx context.Context, instanceID int, stopper InstanceStopper, options Options, clock Clock) error {
	if instanceID <= 0 {
		return fmt.Errorf("Vast instance ID must be positive")
	}
	if clock == nil {
		return fmt.Errorf("clock is required")
	}
	if options.PollInterval <= 0 {
		return fmt.Errorf("poll interval must be positive")
	}
	if options.PollTimeout <= 0 {
		return fmt.Errorf("poll timeout must be positive")
	}
	ctx, cancel := context.WithTimeout(ctx, options.PollTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if options.CheckpointPath != "" {
		unlock, err := lockCheckpoint(options.CheckpointPath)
		if err != nil {
			return err
		}
		defer unlock()
		cp, err := ReadCheckpoint(options.CheckpointPath)
		if err != nil {
			return err
		}
		if cp.InstanceID != instanceID && cp.InstanceID != 0 {
			return fmt.Errorf("recovery checkpoint no longer matches instance %d", instanceID)
		}
		// Stopping preserves the disk and checkpoint for a later resume.
	}
	sawQueuedStart := false
	readStopped := func() (bool, error) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		instance, err := stopper.GetInstance(ctx, instanceID)
		if err != nil {
			return false, err
		}
		if instance.ID != instanceID {
			return false, fmt.Errorf("Vast response did not return instance %d", instanceID)
		}
		// Vast reports an actually stopped container as exited. Desired state,
		// offline/unknown states and absence cannot confirm that compute stopped.
		sawQueuedStart = sawQueuedStart || instance.StartRequested()
		intentCancelled := !sawQueuedStart || strings.EqualFold(instance.IntendedStatus, "stopped")
		return instance.Status == "exited" && !instance.StartRequested() && intentCancelled, nil
	}
	stopped, err := readStopped()
	if err != nil {
		return fmt.Errorf("verify Vast instance before stop: %w", err)
	}
	if stopped {
		return nil
	}
	stopErr := stopper.StopInstance(ctx, instanceID)
	deadline := clock.Now().Add(options.PollTimeout)
	for {
		stopped, err = readStopped()
		if err != nil {
			return fmt.Errorf("verify Vast instance stop: %w", err)
		}
		if stopped {
			return nil
		}
		if !clock.Now().Before(deadline) {
			if stopErr != nil {
				return fmt.Errorf("stop Vast instance remained unconfirmed: %w", stopErr)
			}
			return fmt.Errorf("timed out waiting for Vast instance stop")
		}
		if err := clock.Sleep(ctx, options.PollInterval); err != nil {
			return err
		}
	}
}
