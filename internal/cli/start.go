package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/maximilienGilet/sovereign-kit/internal/catalogui"
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
	Clock        setup.Clock
	PollInterval time.Duration
	PollTimeout  time.Duration
}

func Start(ctx context.Context, output io.Writer, configPath string, deps StartDependencies) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if output == nil {
		output = io.Discard
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
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
	if deps.RunDashboard == nil {
		deps.RunDashboard = runDashboard
	}

	tunnel, err := deps.NewTunnel(ctx, cfg, output)
	if err != nil {
		return err
	}
	if tunnel == nil {
		return errors.New("new tunnel returned nil tunnel")
	}
	if err := tunnel.Start(); err != nil {
		return err
	}
	defer func() { _ = tunnel.Stop() }()

	endpoint := fmt.Sprintf("http://%s:%d", cfg.Route.LocalHost, cfg.Route.LocalPort)
	deadline := deps.Clock.Now().Add(deps.PollTimeout)
	var lastHealthErr error
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case tunnelErr := <-tunnel.Done():
			return tunnelExitError(tunnelErr)
		default:
		}
		if !deps.Clock.Now().Before(deadline) {
			return healthTimeoutError(deps.PollTimeout, lastHealthErr)
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
				return err
			}
			if timerExpired || !deps.Clock.Now().Before(deadline) {
				return healthTimeoutError(deps.PollTimeout, healthErr)
			}
			if healthErr == nil {
				select {
				case tunnelErr := <-tunnel.Done():
					return tunnelExitError(tunnelErr)
				default:
				}
				if _, err := fmt.Fprintln(output, "LIVE: endpoint healthy"); err != nil {
					return err
				}
				return deps.RunDashboard(output)
			}
		case tunnelErr := <-tunnel.Done():
			stopStartTimer(timer)
			cancelHealth()
			<-healthDone
			return tunnelExitError(tunnelErr)
		case <-ctx.Done():
			stopStartTimer(timer)
			cancelHealth()
			<-healthDone
			return ctx.Err()
		case <-timer.C():
			cancelHealth()
			<-healthDone
			return healthTimeoutError(deps.PollTimeout, lastHealthErr)
		}
		if healthErr == nil {
			continue
		}
		lastHealthErr = healthErr
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case tunnelErr := <-tunnel.Done():
			return tunnelExitError(tunnelErr)
		default:
		}
		if !deps.Clock.Now().Before(deadline) {
			return healthTimeoutError(deps.PollTimeout, lastHealthErr)
		}
		if err := deps.Clock.Sleep(ctx, deps.PollInterval); err != nil {
			return err
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

func runDashboard(output io.Writer) error {
	_, err := tea.NewProgram(catalogui.New(catalogui.DefaultEntries()), tea.WithOutput(output)).Run()
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
