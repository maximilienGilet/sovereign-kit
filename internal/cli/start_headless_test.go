package cli

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
)

func TestStartHeadlessKeepsTunnelUntilCancellationWithoutDashboard(t *testing.T) {
	path := t.TempDir() + "/config.toml"
	if err := config.Save(path, config.Studio("gpu", 22, "alice", "/key", "/known")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tunnel := &headlessTunnel{done: make(chan error, 1)}
	var output bytes.Buffer
	done := make(chan error, 1)
	ready := make(chan struct{})
	go func() {
		done <- StartHeadless(ctx, &output, path, StartDependencies{
			NewTunnel:    func(context.Context, config.Config, io.Writer) (Tunnel, error) { return tunnel, nil },
			Healthcheck:  func(context.Context, string) error { close(ready); return nil },
			RunDashboard: func(io.Writer) error { t.Error("headless start opened dashboard"); return nil },
		})
	}()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("connection never checked")
	}
	select {
	case err := <-done:
		t.Fatalf("start ended prematurely: %v", err)
	default:
	}
	cancel()
	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("start did not cancel")
	}
	if !tunnel.stopped {
		t.Fatal("tunnel not stopped")
	}
}

type headlessTunnel struct {
	done    chan error
	stopped bool
}

func (t *headlessTunnel) Start() error       { return nil }
func (t *headlessTunnel) Done() <-chan error { return t.done }
func (t *headlessTunnel) Stop() error        { t.stopped = true; return nil }
