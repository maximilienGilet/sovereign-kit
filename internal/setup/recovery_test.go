package setup

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type recoveryResult struct {
	exists bool
	err    error
}

type fakeDestroyer struct {
	checks       []recoveryResult
	checkCalls   int
	destroyCalls int
	destroyErr   error
	destroyID    int
	destroyBlock <-chan struct{}
	destroyStart chan<- struct{}
	checkBlock   <-chan struct{}
	checkStart   chan<- struct{}
}

func (f *fakeDestroyer) InstanceExists(ctx context.Context, instanceID int) (bool, error) {
	return f.instanceExists(ctx, instanceID)
}

func (f *fakeDestroyer) instanceExists(ctx context.Context, instanceID int) (bool, error) {
	if instanceID != 987 {
		return false, errors.New("wrong instance target")
	}
	if f.checkStart != nil {
		close(f.checkStart)
		f.checkStart = nil
	}
	if f.checkBlock != nil {
		select {
		case <-f.checkBlock:
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
	if f.checkCalls >= len(f.checks) {
		return false, errors.New("unexpected existence check")
	}
	result := f.checks[f.checkCalls]
	f.checkCalls++
	return result.exists, result.err
}

func (f *fakeDestroyer) DestroyInstance(ctx context.Context, instanceID int) error {
	f.destroyCalls++
	f.destroyID = instanceID
	if f.destroyStart != nil {
		close(f.destroyStart)
		f.destroyStart = nil
	}
	if f.destroyBlock != nil {
		select {
		case <-f.destroyBlock:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return f.destroyErr
}

func TestRecoveryDestroyPrecheckAbsenceAvoidsDelete(t *testing.T) {
	destroyer := &fakeDestroyer{checks: []recoveryResult{{exists: false}}}
	recovery := newInstanceRecovery(987, destroyer, testOptions(), &fakeClock{now: time.Unix(0, 0)})
	if err := recovery.Destroy(context.Background()); err != nil {
		t.Fatal(err)
	}
	if destroyer.destroyCalls != 0 {
		t.Fatalf("delete calls = %d", destroyer.destroyCalls)
	}
}

func TestRecoveryDestroyWaitsForExplicitDisappearance(t *testing.T) {
	destroyer := &fakeDestroyer{checks: []recoveryResult{{exists: true}, {exists: true}, {exists: false}}}
	clock := &fakeClock{now: time.Unix(0, 0)}
	recovery := newInstanceRecovery(987, destroyer, testOptions(), clock)
	if err := recovery.Destroy(context.Background()); err != nil {
		t.Fatal(err)
	}
	if destroyer.destroyCalls != 1 || destroyer.destroyID != 987 || len(clock.sleeps) != 1 {
		t.Fatalf("delete calls=%d id=%d sleeps=%v", destroyer.destroyCalls, destroyer.destroyID, clock.sleeps)
	}
}

func TestRecoveryDestroyRejectsStillPresentAtDeadline(t *testing.T) {
	destroyer := &fakeDestroyer{checks: []recoveryResult{{exists: true}, {exists: true}, {exists: true}}}
	clock := &fakeClock{now: time.Unix(0, 0)}
	options := testOptions()
	options.PollTimeout = time.Second
	recovery := newInstanceRecovery(987, destroyer, options, clock)
	if err := recovery.Destroy(context.Background()); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error=%v", err)
	}
}

func TestRecoveryDestroyRechecksAfterAmbiguousDeleteFailure(t *testing.T) {
	destroyer := &fakeDestroyer{checks: []recoveryResult{{exists: true}, {exists: false}}, destroyErr: errors.New("connection lost")}
	recovery := newInstanceRecovery(987, destroyer, testOptions(), &fakeClock{now: time.Unix(0, 0)})
	if err := recovery.Destroy(context.Background()); err != nil {
		t.Fatalf("rechecked disappearance should succeed: %v", err)
	}
}

func TestRecoveryDestroyValidatesBeforeSideEffects(t *testing.T) {
	destroyer := &fakeDestroyer{checks: []recoveryResult{{exists: true}}}
	options := testOptions()
	options.PollInterval = 0
	recovery := newInstanceRecovery(987, destroyer, options, &fakeClock{now: time.Unix(0, 0)})
	if err := recovery.Destroy(context.Background()); err == nil || destroyer.checkCalls != 0 || destroyer.destroyCalls != 0 {
		t.Fatalf("error=%v checks=%d deletes=%d", err, destroyer.checkCalls, destroyer.destroyCalls)
	}
}

func TestRecoveryDestroyRejectsZeroTimeoutBeforeSideEffects(t *testing.T) {
	destroyer := &fakeDestroyer{checks: []recoveryResult{{exists: true}}}
	options := testOptions()
	options.PollTimeout = 0
	recovery := newInstanceRecovery(987, destroyer, options, &fakeClock{now: time.Unix(0, 0)})
	if err := recovery.Destroy(context.Background()); err == nil || destroyer.checkCalls != 0 || destroyer.destroyCalls != 0 {
		t.Fatalf("error=%v checks=%d deletes=%d", err, destroyer.checkCalls, destroyer.destroyCalls)
	}
}

func TestRecoveryDestroyBoundsBlockedPrecheckWithBackgroundCaller(t *testing.T) {
	gate := make(chan struct{})
	started := make(chan struct{})
	destroyer := &fakeDestroyer{checkBlock: gate, checkStart: started}
	options := testOptions()
	options.PollTimeout = 20 * time.Millisecond
	recovery := newInstanceRecovery(987, destroyer, options, &fakeClock{now: time.Unix(0, 0)})
	done := make(chan error, 1)
	go func() { done <- recovery.Destroy(context.Background()) }()
	<-started
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("error=%v", err)
		}
	case <-time.After(time.Second):
		close(gate)
		t.Fatal("blocked precheck ignored recovery timeout")
	}
}

func TestRecoveryDestroyRetryPrechecksAfterPriorCompletion(t *testing.T) {
	destroyer := &fakeDestroyer{checks: []recoveryResult{{exists: true}, {exists: false}, {exists: false}}}
	recovery := newInstanceRecovery(987, destroyer, testOptions(), &fakeClock{now: time.Unix(0, 0)})
	if err := recovery.Destroy(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := recovery.Destroy(context.Background()); err != nil {
		t.Fatal(err)
	}
	if destroyer.destroyCalls != 1 || destroyer.checkCalls != 3 {
		t.Fatalf("delete calls=%d checks=%d", destroyer.destroyCalls, destroyer.checkCalls)
	}
}

func TestRecoveryDestroyStopsOnCancellationBeforeVerification(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	destroyer := &fakeDestroyer{checks: []recoveryResult{{exists: true}}}
	recovery := newInstanceRecovery(987, destroyer, testOptions(), &fakeClock{now: time.Unix(0, 0)})
	if err := recovery.Destroy(ctx); !errors.Is(err, context.Canceled) || destroyer.checkCalls != 0 {
		t.Fatalf("error=%v checks=%d", err, destroyer.checkCalls)
	}
}

func TestRecoveryDestroyFailsClosedWhenExistenceCannotBeEstablished(t *testing.T) {
	for name, lookupErr := range map[string]error{
		"forbidden": errors.New("Vast instance lookup returned HTTP 403"),
		"malformed": errors.New("decode Vast instance lookup response"),
	} {
		t.Run(name, func(t *testing.T) {
			destroyer := &fakeDestroyer{checks: []recoveryResult{{err: lookupErr}}}
			recovery := newInstanceRecovery(987, destroyer, testOptions(), &fakeClock{now: time.Unix(0, 0)})
			if err := recovery.Destroy(context.Background()); err == nil || destroyer.destroyCalls != 0 {
				t.Fatalf("error=%v delete calls=%d", err, destroyer.destroyCalls)
			}
		})
	}
}

func TestRecoveryDestroyRejectsConcurrentAttempt(t *testing.T) {
	gate := make(chan struct{})
	started := make(chan struct{})
	destroyer := &fakeDestroyer{checks: []recoveryResult{{exists: true}, {exists: false}}, destroyBlock: gate, destroyStart: started}
	recovery := newInstanceRecovery(987, destroyer, testOptions(), &fakeClock{now: time.Unix(0, 0)})
	done := make(chan error, 1)
	go func() { done <- recovery.Destroy(context.Background()) }()
	<-started
	if err := recovery.Destroy(context.Background()); err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("concurrent error=%v", err)
	}
	close(gate)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
