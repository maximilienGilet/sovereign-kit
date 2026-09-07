package setup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

type stoppingAPI struct {
	fakeDestroyer
	instances  []vast.Instance
	getErr     error
	stopErr    error
	stopCalls  int
	getCalls   int
	getBlock   <-chan struct{}
	getStarted chan<- struct{}
}

func (a *stoppingAPI) GetInstance(ctx context.Context, id int) (vast.Instance, error) {
	if id != 987 {
		return vast.Instance{}, errors.New("wrong target")
	}
	a.getCalls++
	if a.getStarted != nil {
		a.getStarted <- struct{}{}
	}
	if a.getBlock != nil {
		select {
		case <-a.getBlock:
		case <-ctx.Done():
			return vast.Instance{}, ctx.Err()
		}
	}
	if a.getErr != nil {
		return vast.Instance{}, a.getErr
	}
	if len(a.instances) == 0 {
		return vast.Instance{}, errors.New("no instance response")
	}
	i := a.instances[0]
	if len(a.instances) > 1 {
		a.instances = a.instances[1:]
	}
	return i, nil
}

func TestRecoveryStopPreservesCheckpointAndRespectsOwnership(t *testing.T) {
	for _, mode := range []string{"matching", "mismatched", "locked", "missing"} {
		t.Run(mode, func(t *testing.T) {
			options := testOptions()
			options.CheckpointPath = filepath.Join(t.TempDir(), "pending.json")
			id := 987
			if mode == "mismatched" {
				id = 123
			}
			cp := Checkpoint{Version: 1, InstanceID: id, Recipe: validRecipe(), IdentityFile: "identity", KnownHostsDir: "hosts", Phase: "created"}
			if mode != "missing" {
				if err := writeCheckpoint(options.CheckpointPath, cp); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(options.CheckpointPath)
			if mode == "locked" {
				unlock, err := lockCheckpoint(options.CheckpointPath)
				if err != nil {
					t.Fatal(err)
				}
				defer unlock()
			}
			a := &stoppingAPI{instances: []vast.Instance{{ID: 987, Status: "running"}, {ID: 987, Status: "exited"}}}
			err := NewInstanceRecovery(987, a, options, &fakeClock{}).Stop(context.Background())
			if (err == nil) != (mode == "matching") {
				t.Fatalf("error=%v", err)
			}
			if mode != "matching" && (a.stopCalls != 0 || a.getCalls != 0) {
				t.Fatal("unsafe checkpoint allowed provider access")
			}
			after, _ := os.ReadFile(options.CheckpointPath)
			if string(before) != string(after) {
				t.Fatal("stop altered checkpoint")
			}
		})
	}
}

func TestRecoveryStopBoundsBlockedLookupAndExcludesConcurrentDestroy(t *testing.T) {
	options := testOptions()
	options.PollTimeout = 30 * time.Millisecond
	started := make(chan struct{}, 1)
	a := &stoppingAPI{getStarted: started, getBlock: make(chan struct{})}
	recovery := NewInstanceRecovery(987, a, options, &fakeClock{})
	done := make(chan error, 1)
	go func() { done <- recovery.Stop(context.Background()) }()
	<-started
	if err := recovery.Destroy(context.Background()); err == nil {
		t.Fatal("concurrent destroy accepted")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("stop lookup not bounded")
	}
}

func TestRecoveryWithoutStopSupportOmitsStopCapability(t *testing.T) {
	if NewInstanceRecovery(987, &fakeDestroyer{}, testOptions(), &fakeClock{}).Stop != nil {
		t.Fatal("unsupported stop offered")
	}
}
func (a *stoppingAPI) StopInstance(ctx context.Context, id int) error {
	if id != 987 {
		return errors.New("wrong stop target")
	}
	a.stopCalls++
	return a.stopErr
}

func TestRecoveryStopVerifiesExactActualState(t *testing.T) {
	for _, tc := range []struct {
		name      string
		instances []vast.Instance
		stopErr   error
		wantErr   bool
		calls     int
	}{
		{"wait for exited", []vast.Instance{{ID: 987, Status: "running"}, {ID: 987, Status: "running"}, {ID: 987, Status: "exited"}}, nil, false, 1},
		{"already exited", []vast.Instance{{ID: 987, Status: "exited"}}, nil, false, 0},
		{"cancel queued start", []vast.Instance{{ID: 987, Status: "exited", IntendedStatus: "running"}, {ID: 987, Status: "exited", IntendedStatus: "running"}, {ID: 987, Status: "exited", IntendedStatus: "stopped"}}, nil, false, 1},
		{"next state still queued", []vast.Instance{{ID: 987, Status: "exited", NextState: "running"}}, nil, true, 1},
		{"missing intent cannot confirm queue cancellation", []vast.Instance{{ID: 987, Status: "exited", IntendedStatus: "running"}, {ID: 987, Status: "exited"}}, nil, true, 1},
		{"ambiguous accepted", []vast.Instance{{ID: 987, Status: "running"}, {ID: 987, Status: "exited"}}, errors.New("connection lost"), false, 1},
		{"ambiguous unconfirmed", []vast.Instance{{ID: 987, Status: "running"}}, errors.New("connection lost"), true, 1},
		{"offline is not stopped", []vast.Instance{{ID: 987, Status: "offline"}}, nil, true, 1},
		{"unknown is not stopped", []vast.Instance{{ID: 987, Status: "unknown"}}, nil, true, 1},
		{"wrong precheck id", []vast.Instance{{ID: 123, Status: "exited"}}, nil, true, 0},
		{"wrong confirmation id", []vast.Instance{{ID: 987, Status: "running"}, {ID: 123, Status: "exited"}}, nil, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &stoppingAPI{instances: tc.instances, stopErr: tc.stopErr}
			clock := &fakeClock{now: time.Unix(0, 0)}
			err := NewInstanceRecovery(987, a, testOptions(), clock).Stop(context.Background())
			if (err != nil) != tc.wantErr || a.stopCalls != tc.calls {
				t.Fatalf("err=%v stop calls=%d", err, a.stopCalls)
			}
		})
	}
}

func TestRecoveryStopValidatesBeforeNetwork(t *testing.T) {
	for _, invalid := range []string{"id", "clock", "interval", "timeout", "canceled"} {
		t.Run(invalid, func(t *testing.T) {
			a := &stoppingAPI{}
			id := 987
			var clock Clock = &fakeClock{}
			opts := testOptions()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch invalid {
			case "id":
				id = 0
			case "clock":
				clock = nil
			case "interval":
				opts.PollInterval = 0
			case "timeout":
				opts.PollTimeout = 0
			case "canceled":
				cancel()
			}
			if err := NewInstanceRecovery(id, a, opts, clock).Stop(ctx); err == nil || a.getCalls != 0 || a.stopCalls != 0 {
				t.Fatalf("err=%v calls=%d/%d", err, a.getCalls, a.stopCalls)
			}
		})
	}
}
