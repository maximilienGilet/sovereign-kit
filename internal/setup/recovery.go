package setup

import (
	"context"
	"fmt"
	"sync"
)

// InstanceRecovery provides narrowly scoped lifecycle actions for one instance.
type InstanceRecovery struct {
	InstanceID int
	Stop       func(context.Context) error
	Destroy    func(context.Context) error
}

// RecoveryObserver optionally receives the recovery capability immediately
// after a successful instance creation.
type RecoveryObserver interface {
	InstanceCreated(InstanceRecovery)
}

// InstanceDestroyer is implemented by Vast clients that can safely verify and
// destroy an exact instance. It remains separate from the required VastAPI.
type InstanceDestroyer interface {
	DestroyInstance(context.Context, int) error
	InstanceExists(context.Context, int) (bool, error)
}

// NewInstanceRecovery binds lifecycle actions to one exact instance. Stop is
// available only when the client can both request and verify its actual state.
func NewInstanceRecovery(instanceID int, destroyer InstanceDestroyer, options Options, clock Clock) InstanceRecovery {
	return newInstanceRecovery(instanceID, destroyer, options, clock)
}

func newInstanceRecovery(instanceID int, destroyer InstanceDestroyer, options Options, clock Clock) InstanceRecovery {
	var mu sync.Mutex
	running := false
	recovery := InstanceRecovery{
		InstanceID: instanceID,
		Destroy: func(ctx context.Context) (resultErr error) {
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
				defer func() {
					if resultErr == nil && cp.InstanceID == instanceID {
						resultErr = removeCheckpoint(options.CheckpointPath)
					}
				}()
			}
			mu.Lock()
			if running {
				mu.Unlock()
				return fmt.Errorf("Vast instance destruction is already in progress")
			}
			running = true
			mu.Unlock()
			defer func() {
				mu.Lock()
				running = false
				mu.Unlock()
			}()

			if instanceID <= 0 {
				return fmt.Errorf("Vast instance ID must be positive")
			}
			if destroyer == nil {
				return fmt.Errorf("Vast instance destroyer is required")
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
			boundedCtx, cancel := context.WithTimeout(ctx, options.PollTimeout)
			defer cancel()
			ctx = boundedCtx
			if err := ctx.Err(); err != nil {
				return err
			}

			exists, err := destroyer.InstanceExists(ctx, instanceID)
			if err != nil {
				return fmt.Errorf("verify Vast instance before destruction: %w", err)
			}
			if !exists {
				return nil
			}
			if err := destroyer.DestroyInstance(ctx, instanceID); err != nil {
				// A transport failure can occur after Vast accepted the DELETE.
				exists, verifyErr := destroyer.InstanceExists(ctx, instanceID)
				if verifyErr == nil && !exists {
					return nil
				}
				return fmt.Errorf("destroy Vast instance: %w", err)
			}

			deadline := clock.Now().Add(options.PollTimeout)
			for {
				if err := ctx.Err(); err != nil {
					return err
				}
				exists, err := destroyer.InstanceExists(ctx, instanceID)
				if err != nil {
					return fmt.Errorf("verify Vast instance destruction: %w", err)
				}
				if !exists {
					return nil
				}
				if !clock.Now().Before(deadline) {
					return fmt.Errorf("timed out waiting for Vast instance destruction")
				}
				if err := clock.Sleep(ctx, options.PollInterval); err != nil {
					return err
				}
			}
		},
	}
	if stopper, ok := destroyer.(InstanceStopper); ok {
		recovery.Stop = func(ctx context.Context) error {
			mu.Lock()
			if running {
				mu.Unlock()
				return fmt.Errorf("Vast instance lifecycle action is already in progress")
			}
			running = true
			mu.Unlock()
			defer func() { mu.Lock(); running = false; mu.Unlock() }()
			return stopAndVerifyInstance(ctx, instanceID, stopper, options, clock)
		}
	}
	return recovery
}

func notifyRecovery(operator Operator, recovery InstanceRecovery) {
	if observer, ok := operator.(RecoveryObserver); ok {
		observer.InstanceCreated(recovery)
	}
}
