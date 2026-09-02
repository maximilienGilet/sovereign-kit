package cli

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
)

type startTestTunnel struct {
	events    *[]string
	done      chan error
	startErr  error
	stopErr   error
	stopCount int
}

func (t *startTestTunnel) Start() error {
	*t.events = append(*t.events, "tunnel-start")
	return t.startErr
}

func (t *startTestTunnel) Done() <-chan error {
	return t.done
}

func (t *startTestTunnel) Stop() error {
	t.stopCount++
	*t.events = append(*t.events, "tunnel-stop")
	return t.stopErr
}

type startTestClock struct {
	events   *[]string
	now      time.Time
	sleeps   []time.Duration
	advance  bool
	sleepErr error
}

func (c *startTestClock) Now() time.Time {
	return c.now
}

func (c *startTestClock) Sleep(ctx context.Context, duration time.Duration) error {
	*c.events = append(*c.events, "sleep")
	c.sleeps = append(c.sleeps, duration)
	if c.advance {
		c.now = c.now.Add(duration)
	}
	if c.sleepErr != nil {
		return c.sleepErr
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func startTestConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg := config.Studio("gpu.example.test", 22022, "sovkit", "/tmp/identity", "/tmp/known_hosts")
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	return path
}

func startTestDependencies(tunnel *startTestTunnel, clock *startTestClock, health func(context.Context, string) error, dashboard func(io.Writer) error) StartDependencies {
	return StartDependencies{
		NewTunnel: func(context.Context, config.Config, io.Writer) (Tunnel, error) {
			return tunnel, nil
		},
		Healthcheck:  health,
		RunDashboard: dashboard,
		Clock:        clock,
		PollInterval: 5 * time.Second,
		PollTimeout:  30 * time.Minute,
	}
}

func TestStartWaitsForHealthBeforeDashboard(t *testing.T) {
	events := []string{}
	clock := &startTestClock{events: &events, now: time.Unix(0, 0)}
	tunnel := &startTestTunnel{events: &events, done: make(chan error, 1)}
	attempts := 0
	deps := startTestDependencies(tunnel, clock, func(_ context.Context, endpoint string) error {
		if endpoint != "http://127.0.0.1:30000" {
			t.Fatalf("health endpoint = %q", endpoint)
		}
		attempts++
		if attempts == 1 {
			events = append(events, "health-fail")
			return errors.New("warming up")
		}
		events = append(events, "health-pass")
		return nil
	}, func(_ io.Writer) error {
		events = append(events, "dashboard")
		return nil
	})

	if err := Start(context.Background(), io.Discard, startTestConfig(t), deps); err != nil {
		t.Fatal(err)
	}
	want := []string{"tunnel-start", "health-fail", "sleep", "health-pass", "dashboard", "tunnel-stop"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
	if !reflect.DeepEqual(clock.sleeps, []time.Duration{5 * time.Second}) {
		t.Fatalf("sleeps = %#v", clock.sleeps)
	}
}

func TestStartStopsTunnelWhenDashboardExits(t *testing.T) {
	events := []string{}
	clock := &startTestClock{events: &events, now: time.Unix(0, 0)}
	tunnel := &startTestTunnel{events: &events, done: make(chan error, 1)}
	dashboardErr := errors.New("dashboard closed")
	deps := startTestDependencies(tunnel, clock, func(_ context.Context, _ string) error {
		events = append(events, "health-pass")
		return nil
	}, func(_ io.Writer) error {
		events = append(events, "dashboard")
		return dashboardErr
	})

	err := Start(context.Background(), io.Discard, startTestConfig(t), deps)
	if !errors.Is(err, dashboardErr) {
		t.Fatalf("error = %v, want %v", err, dashboardErr)
	}
	want := []string{"tunnel-start", "health-pass", "dashboard", "tunnel-stop"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
	if tunnel.stopCount != 1 {
		t.Fatalf("stop count = %d, want 1", tunnel.stopCount)
	}
}

func TestStartStopsTunnelWhenHealthTimesOut(t *testing.T) {
	events := []string{}
	clock := &startTestClock{events: &events, now: time.Unix(0, 0), advance: true}
	tunnel := &startTestTunnel{events: &events, done: make(chan error, 1)}
	deps := startTestDependencies(tunnel, clock, func(_ context.Context, _ string) error {
		events = append(events, "health-fail")
		return errors.New("not ready")
	}, func(_ io.Writer) error {
		events = append(events, "dashboard")
		return nil
	})
	deps.PollTimeout = 10 * time.Second

	err := Start(context.Background(), io.Discard, startTestConfig(t), deps)
	if err == nil || !strings.Contains(err.Error(), "health") {
		t.Fatalf("error = %v, want health timeout", err)
	}
	if tunnel.stopCount != 1 {
		t.Fatalf("stop count = %d, want 1", tunnel.stopCount)
	}
	if strings.Contains(strings.Join(events, ","), "dashboard") {
		t.Fatalf("dashboard ran after health timeout: %#v", events)
	}
	if !reflect.DeepEqual(clock.sleeps, []time.Duration{5 * time.Second, 5 * time.Second}) {
		t.Fatalf("sleeps = %#v", clock.sleeps)
	}
}

func TestStartFailsImmediatelyWhenTunnelExits(t *testing.T) {
	events := []string{}
	clock := &startTestClock{events: &events, now: time.Unix(0, 0)}
	tunnelErr := errors.New("ssh exited")
	tunnel := &startTestTunnel{events: &events, done: make(chan error, 1)}
	tunnel.done <- tunnelErr
	deps := startTestDependencies(tunnel, clock, func(_ context.Context, _ string) error {
		events = append(events, "health")
		return nil
	}, func(_ io.Writer) error {
		events = append(events, "dashboard")
		return nil
	})

	err := Start(context.Background(), io.Discard, startTestConfig(t), deps)
	if !errors.Is(err, tunnelErr) {
		t.Fatalf("error = %v, want %v", err, tunnelErr)
	}
	want := []string{"tunnel-start", "tunnel-stop"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

func TestStartNeverTreatsHealthAsMeasuredPerformance(t *testing.T) {
	events := []string{}
	clock := &startTestClock{events: &events, now: time.Unix(0, 0)}
	tunnel := &startTestTunnel{events: &events, done: make(chan error, 1)}
	var output strings.Builder
	deps := startTestDependencies(tunnel, clock, func(_ context.Context, _ string) error {
		return nil
	}, func(io.Writer) error {
		return nil
	})

	if err := Start(context.Background(), &output, startTestConfig(t), deps); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "LIVE: endpoint healthy") {
		t.Fatalf("output = %q, want live status", output.String())
	}
	for _, forbidden := range []string{"MEASURED", "tokens/second", "latency", "throughput"} {
		if strings.Contains(strings.ToLower(output.String()), strings.ToLower(forbidden)) {
			t.Fatalf("output = %q contains %q", output.String(), forbidden)
		}
	}
}

func TestStartLoadsConfigBeforeCreatingTunnel(t *testing.T) {
	events := []string{}
	clock := &startTestClock{events: &events, now: time.Unix(0, 0)}
	tunnel := &startTestTunnel{events: &events, done: make(chan error, 1)}
	deps := startTestDependencies(tunnel, clock, func(_ context.Context, _ string) error { return nil }, func(io.Writer) error { return nil })

	if err := Start(context.Background(), io.Discard, filepath.Join(t.TempDir(), "missing.toml"), deps); err == nil {
		t.Fatal("Start() error = nil, want config load error")
	}
	if tunnel.stopCount != 0 {
		t.Fatalf("stop count = %d, want 0 before tunnel creation", tunnel.stopCount)
	}
}
func TestStartAbortsBlockedHealthAtPollTimeout(t *testing.T) {
	events := []string{}
	clock := &startTestClock{events: &events, now: time.Unix(0, 0)}
	tunnel := &startTestTunnel{events: &events, done: make(chan error, 1)}
	healthStarted := make(chan struct{})
	healthReleased := make(chan struct{})
	deps := startTestDependencies(tunnel, clock, func(ctx context.Context, _ string) error {
		close(healthStarted)
		<-ctx.Done()
		close(healthReleased)
		return ctx.Err()
	}, func(_ io.Writer) error {
		t.Fatal("dashboard ran after blocked health timeout")
		return nil
	})
	deps.PollTimeout = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-healthStarted
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := Start(ctx, io.Discard, startTestConfig(t), deps)
	if err == nil || !strings.Contains(err.Error(), "healthcheck timed out") {
		t.Fatalf("error = %v, want healthcheck timeout", err)
	}
	select {
	case <-healthReleased:
	case <-time.After(time.Second):
		t.Fatal("healthcheck did not release after timeout")
	}
	if tunnel.stopCount != 1 {
		t.Fatalf("stop count = %d, want 1", tunnel.stopCount)
	}
}

func TestStartAbortsBlockedHealthWhenTunnelExits(t *testing.T) {
	events := []string{}
	clock := &startTestClock{events: &events, now: time.Unix(0, 0)}
	tunnelErr := errors.New("ssh exited")
	tunnel := &startTestTunnel{events: &events, done: make(chan error, 1)}
	healthStarted := make(chan struct{})
	healthReleased := make(chan struct{})
	deps := startTestDependencies(tunnel, clock, func(ctx context.Context, _ string) error {
		close(healthStarted)
		<-ctx.Done()
		close(healthReleased)
		return ctx.Err()
	}, func(_ io.Writer) error {
		t.Fatal("dashboard ran after tunnel exit")
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-healthStarted
		time.Sleep(10 * time.Millisecond)
		tunnel.done <- tunnelErr
		time.Sleep(40 * time.Millisecond)
		cancel()
	}()

	err := Start(ctx, io.Discard, startTestConfig(t), deps)
	if !errors.Is(err, tunnelErr) {
		t.Fatalf("error = %v, want %v", err, tunnelErr)
	}
	select {
	case <-healthReleased:
	case <-time.After(time.Second):
		t.Fatal("healthcheck did not release after tunnel exit")
	}
	if tunnel.stopCount != 1 {
		t.Fatalf("stop count = %d, want 1", tunnel.stopCount)
	}
}
type expiredStartTimer struct{}

func (expiredStartTimer) C() <-chan time.Time {
	return nil
}

func (expiredStartTimer) Stop() bool {
	return false
}

func TestStartRejectsHealthResultAfterTimerExpiry(t *testing.T) {
	events := []string{}
	clock := &startTestClock{events: &events, now: time.Unix(0, 0)}
	tunnel := &startTestTunnel{events: &events, done: make(chan error, 1)}
	var output strings.Builder
	deps := startTestDependencies(tunnel, clock, func(_ context.Context, _ string) error {
		return nil
	}, func(_ io.Writer) error {
		t.Fatal("dashboard ran after timer expiry")
		return nil
	})
	previousTimer := newStartTimer
	t.Cleanup(func() { newStartTimer = previousTimer })
	newStartTimer = func(time.Duration) startTimer {
		return expiredStartTimer{}
	}

	err := Start(context.Background(), &output, startTestConfig(t), deps)
	if err == nil || !strings.Contains(err.Error(), "healthcheck timed out") {
		t.Fatalf("error = %v, want healthcheck timeout", err)
	}
	if strings.Contains(output.String(), "LIVE: endpoint healthy") {
		t.Fatalf("output = %q, want no LIVE status", output.String())
	}
	if tunnel.stopCount != 1 {
		t.Fatalf("stop count = %d, want 1", tunnel.stopCount)
	}
}
