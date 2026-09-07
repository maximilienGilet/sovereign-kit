package cli

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// Catch accidental ownership retention or dashboard execution by the connection service.
func TestConnectTransfersHealthyTunnelWithoutDashboard(t *testing.T) {
	events := []string{}
	tunnel := &startTestTunnel{events: &events, done: make(chan error, 1)}
	deps := startTestDependencies(tunnel, &startTestClock{events: &events, now: time.Unix(0, 0)}, func(context.Context, string) error { return nil }, func(io.Writer) error { t.Fatal("Connect launched dashboard"); return nil })
	got, err := Connect(context.Background(), io.Discard, startTestConfig(t), deps)
	if err != nil || got != tunnel || tunnel.stopCount != 0 {
		t.Fatalf("tunnel=%v error=%v stops=%d", got, err, tunnel.stopCount)
	}
	if err := got.Stop(); err != nil {
		t.Fatal(err)
	}
	if tunnel.stopCount != 1 {
		t.Fatal("caller could not stop owned tunnel")
	}
}

// Catch tunnel leaks on each connection failure while an in-flight health check is cancelled.
func TestConnectStopsTunnelOnFailure(t *testing.T) {
	for _, reason := range []string{"timeout", "cancel", "exit"} {
		t.Run(reason, func(t *testing.T) {
			events := []string{}
			tunnel := &startTestTunnel{events: &events, done: make(chan error, 1)}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			started := make(chan struct{})
			deps := startTestDependencies(tunnel, &startTestClock{events: &events, now: time.Unix(0, 0)}, func(ctx context.Context, _ string) error { close(started); <-ctx.Done(); return ctx.Err() }, func(io.Writer) error { t.Fatal("dashboard ran"); return nil })
			if reason == "timeout" {
				deps.PollTimeout = 10 * time.Millisecond
			}
			if reason != "timeout" {
				go func() {
					<-started
					if reason == "cancel" {
						cancel()
					} else {
						tunnel.done <- errors.New("ssh exited")
					}
				}()
			}
			got, err := Connect(ctx, io.Discard, startTestConfig(t), deps)
			if got != nil || err == nil || tunnel.stopCount != 1 {
				t.Fatalf("tunnel=%v error=%v stops=%d", got, err, tunnel.stopCount)
			}
			if reason == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("error=%v", err)
			}
			if reason == "timeout" && !strings.Contains(err.Error(), "timed out") {
				t.Fatalf("error=%v", err)
			}
			if reason == "exit" && !strings.Contains(err.Error(), "ssh exited") {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
