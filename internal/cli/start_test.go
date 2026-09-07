package cli

import (
	"context"
	"io"
	"path/filepath"
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

func startTestDependencies(tunnel *startTestTunnel, clock *startTestClock, health func(context.Context, string) error) StartDependencies {
	return StartDependencies{
		NewTunnel: func(context.Context, config.Config, io.Writer) (Tunnel, error) {
			return tunnel, nil
		},
		Healthcheck:  health,
		Clock:        clock,
		PollInterval: 5 * time.Second,
		PollTimeout:  30 * time.Minute,
	}
}
