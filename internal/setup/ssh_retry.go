package setup

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Only collection failures are retried, never fingerprint validation or trust
// decisions. A running provider instance can precede readiness of its SSH relay.
type hostKeyCollectionError struct{ cause error }

func (e *hostKeyCollectionError) Error() string { return "scan host keys: " + e.cause.Error() }
func (e *hostKeyCollectionError) Unwrap() error { return e.cause }

func waitForHostKeys(ctx context.Context, host string, port, instanceID int, deps Dependencies) (HostKeys, error) {
	const budget = 2 * time.Minute
	const interval = 5 * time.Second
	deadline := deps.Clock.Now().Add(budget)
	waitCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	var last error
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return HostKeys{}, err
		}
		remaining := deadline.Sub(deps.Clock.Now())
		if remaining <= 0 || waitCtx.Err() != nil {
			return HostKeys{}, fmt.Errorf("timed out waiting for SSH after %s; resume this instance to retry: %w", budget, last)
		}
		if observer, ok := deps.Operator.(ActivityObserver); ok {
			observer.InstanceActivity(Activity{InstanceID: instanceID, Stage: ProgressHostKeys, Status: "SSH", Message: fmt.Sprintf("Reading SSH host keys · attempt %d · %ds remaining", attempt, int(remaining.Seconds())), CheckedAt: deps.Clock.Now()})
		}
		scanCtx, stop := context.WithTimeout(waitCtx, min(remaining, 15*time.Second))
		keys, err := deps.HostKeyScanner.Scan(scanCtx, host, port)
		scanErr := scanCtx.Err()
		stop()
		if ctx.Err() != nil {
			return HostKeys{}, ctx.Err()
		}
		if err == nil && scanErr == nil {
			return keys, nil
		}
		if err == nil {
			err = scanErr
		}
		last = err
		var collection *hostKeyCollectionError
		if !errors.As(err, &collection) && !errors.Is(err, context.DeadlineExceeded) {
			return HostKeys{}, err
		}
		remaining = deadline.Sub(deps.Clock.Now())
		if remaining <= 0 || waitCtx.Err() != nil {
			continue
		}
		if observer, ok := deps.Operator.(ActivityObserver); ok {
			observer.InstanceActivity(Activity{InstanceID: instanceID, Stage: ProgressHostKeys, Status: "SSH", Message: fmt.Sprintf("SSH not ready · attempt %d failed · retrying in %ds", attempt, int(min(interval, remaining).Seconds())), CheckedAt: deps.Clock.Now()})
		}
		if err := deps.Clock.Sleep(waitCtx, min(interval, remaining)); err != nil {
			if ctx.Err() != nil {
				return HostKeys{}, ctx.Err()
			}
			return HostKeys{}, fmt.Errorf("timed out waiting for SSH; resume this instance to retry: %w", last)
		}
	}
}
