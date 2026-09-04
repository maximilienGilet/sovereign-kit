package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/clientprofile"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/route"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
)

type startTimer interface {
	C() <-chan time.Time
	Stop() bool
}

type realStartTimer struct {
	timer *time.Timer
}

func (t realStartTimer) C() <-chan time.Time {
	return t.timer.C
}

func (t realStartTimer) Stop() bool {
	return t.timer.Stop()
}

var newStartTimer = func(duration time.Duration) startTimer {
	return realStartTimer{timer: time.NewTimer(duration)}
}

type Tunnel interface {
	Start() error
	Done() <-chan error
	Stop() error
}

type StartDependencies struct {
	NewTunnel    func(context.Context, config.Config, io.Writer) (Tunnel, error)
	Healthcheck  func(context.Context, string) error
	RunDashboard func(io.Writer) error
	Discover     func(context.Context, string, clientprofile.Metadata) clientprofile.Endpoint
	Clock        setup.Clock
	PollInterval time.Duration
	PollTimeout  time.Duration
}

func Start(ctx context.Context, output io.Writer, configPath string, deps StartDependencies) error {
	if deps.RunDashboard == nil {
		return RunApplication(ctx, os.Stdin, output, configPath, "root", "start", ApplicationDependencies{Start: deps})
	}
	if output == nil {
		output = io.Discard
	}
	tunnel, err := Connect(ctx, output, configPath, deps)
	if err != nil {
		return err
	}
	defer func() { _ = tunnel.Stop() }()
	return deps.RunDashboard(output)
}

// StartHeadless owns only the local connection. Clients are launched explicitly
// in another terminal; cancellation never destroys the remote instance.
func StartHeadless(ctx context.Context, output io.Writer, configPath string, deps StartDependencies) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if output == nil {
		output = io.Discard
	}
	tunnel, err := Connect(ctx, output, configPath, deps)
	if err != nil {
		return err
	}
	defer func() { _ = tunnel.Stop() }()
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	discover := deps.Discover
	if discover == nil {
		discover = clientprofile.Discover
	}
	endpoint := discover(ctx, fmt.Sprintf("http://%s:%d/v1", cfg.Route.LocalHost, cfg.Route.LocalPort), clientprofile.Metadata{ID: cfg.Model.ID, ContextWindow: cfg.Model.ContextWindow, MaxTokens: cfg.Model.MaxTokens})
	if _, err := fmt.Fprintf(output, "Base URL: %s\nModel: %s\n", endpoint.BaseURL, endpoint.ID); err != nil {
		return err
	}
	if endpoint.Problem != "" {
		if _, err := fmt.Fprintln(output, endpoint.Problem); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(output, "Connection active. Launch your client explicitly in another terminal. Ctrl+C disconnects locally; remote instance billing may continue."); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return nil
	case err := <-tunnel.Done():
		return tunnelExitError(err)
	}
}

// Connect returns a healthy running tunnel. The caller must stop it on success.
func Connect(ctx context.Context, output io.Writer, configPath string, deps StartDependencies) (Tunnel, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if output == nil {
		output = io.Discard
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("load configuration: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if deps.Clock == nil {
		deps.Clock = setup.RealClock{}
	}
	if deps.PollInterval <= 0 {
		deps.PollInterval = 5 * time.Second
	}
	if deps.PollTimeout <= 0 {
		deps.PollTimeout = 30 * time.Minute
	}
	if deps.NewTunnel == nil {
		deps.NewTunnel = newCommandTunnel
	}
	if deps.Healthcheck == nil {
		deps.Healthcheck = route.Healthcheck
	}

	tunnel, err := deps.NewTunnel(ctx, cfg, output)
	if err != nil {
		return nil, err
	}
	if tunnel == nil {
		return nil, errors.New("new tunnel returned nil tunnel")
	}
	if err := tunnel.Start(); err != nil {
		return nil, err
	}
	owned := false
	defer func() {
		if !owned {
			_ = tunnel.Stop()
		}
	}()

	endpoint := fmt.Sprintf("http://%s:%d", cfg.Route.LocalHost, cfg.Route.LocalPort)
	deadline := deps.Clock.Now().Add(deps.PollTimeout)
	var lastHealthErr error
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		select {
		case tunnelErr := <-tunnel.Done():
			return nil, tunnelExitError(tunnelErr)
		default:
		}
		if !deps.Clock.Now().Before(deadline) {
			return nil, healthTimeoutError(deps.PollTimeout, lastHealthErr)
		}
		remaining := deadline.Sub(deps.Clock.Now())
		healthCtx, cancelHealth := context.WithCancel(ctx)
		healthDone := make(chan error, 1)
		go func() {
			healthDone <- deps.Healthcheck(healthCtx, endpoint)
		}()
		timer := newStartTimer(remaining)
		var healthErr error
		select {
		case healthErr = <-healthDone:
			timerExpired := !timer.Stop()
			if timerExpired {
				select {
				case <-timer.C():
				default:
				}
			}
			cancelHealth()
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if timerExpired || !deps.Clock.Now().Before(deadline) {
				return nil, healthTimeoutError(deps.PollTimeout, healthErr)
			}
			if healthErr == nil {
				select {
				case tunnelErr := <-tunnel.Done():
					return nil, tunnelExitError(tunnelErr)
				default:
				}
				if _, err := fmt.Fprintln(output, "LIVE: endpoint healthy"); err != nil {
					return nil, err
				}
				owned = true
				return tunnel, nil
			}
		case tunnelErr := <-tunnel.Done():
			stopStartTimer(timer)
			cancelHealth()
			<-healthDone
			return nil, tunnelExitError(tunnelErr)
		case <-ctx.Done():
			stopStartTimer(timer)
			cancelHealth()
			<-healthDone
			return nil, ctx.Err()
		case <-timer.C():
			cancelHealth()
			<-healthDone
			return nil, healthTimeoutError(deps.PollTimeout, lastHealthErr)
		}
		if healthErr == nil {
			continue
		}
		lastHealthErr = healthErr
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		select {
		case tunnelErr := <-tunnel.Done():
			return nil, tunnelExitError(tunnelErr)
		default:
		}
		if !deps.Clock.Now().Before(deadline) {
			return nil, healthTimeoutError(deps.PollTimeout, lastHealthErr)
		}
		if err := deps.Clock.Sleep(ctx, deps.PollInterval); err != nil {
			return nil, err
		}
	}
}

func stopStartTimer(timer startTimer) {
	if !timer.Stop() {
		select {
		case <-timer.C():
		default:
		}
	}
}

func tunnelExitError(err error) error {
	if err == nil {
		return errors.New("tunnel exited unexpectedly")
	}
	return fmt.Errorf("tunnel exited: %w", err)
}

func healthTimeoutError(timeout time.Duration, last error) error {
	if last == nil {
		return fmt.Errorf("healthcheck timed out after %s", timeout)
	}
	return fmt.Errorf("healthcheck timed out after %s: %w", timeout, last)
}

func RunDashboard(input io.Reader, output io.Writer) error {
	_, err := fmt.Fprintln(output, "Run sovkit start to open the connected endpoint dashboard. Integrations are optional and never launched automatically.")
	return err
}

type commandTunnel struct {
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	done     chan error
	exited   chan error
	stopOnce sync.Once
	stopErr  error
}

func newCommandTunnel(parent context.Context, cfg config.Config, output io.Writer) (Tunnel, error) {
	ctx, cancel := context.WithCancel(parent)
	cmd, err := route.CommandContext(ctx, cfg)
	if err != nil {
		cancel()
		return nil, err
	}
	cmd.Stdout = output
	cmd.Stderr = output
	return &commandTunnel{
		cmd:    cmd,
		cancel: cancel,
		done:   make(chan error, 1),
		exited: make(chan error, 1),
	}, nil
}

func (t *commandTunnel) Start() error {
	if err := t.cmd.Start(); err != nil {
		t.cancel()
		return err
	}
	go func() {
		err := t.cmd.Wait()
		t.done <- err
		t.exited <- err
	}()
	return nil
}

func (t *commandTunnel) Done() <-chan error {
	return t.exited
}

func (t *commandTunnel) Stop() error {
	t.stopOnce.Do(func() {
		t.cancel()
		t.stopErr = <-t.done
	})
	return t.stopErr
}
