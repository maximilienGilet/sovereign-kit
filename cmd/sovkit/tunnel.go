package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/cli"
	"github.com/maximilienGilet/sovereign-kit/internal/route"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"github.com/maximilienGilet/sovereign-kit/internal/state"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

// noActiveError names the way forward when nothing owns an instance.
func noActiveError(dir string, store state.Store) error {
	return fmt.Errorf("no active deployment.%s", legacyHint(dir, store))
}

// destroyHint names the recovery command for a dead-end deployment.
func destroyHint(id string) string {
	return fmt.Sprintf("run `sovkit destroy %s` to clean up", id)
}

// resolveDeployment finds a deployment by id, or the active one when id is
// empty.
func resolveDeployment(dir string, store state.Store, id string) (state.Deployment, error) {
	if id != "" {
		deployment, ok := store.Get(id)
		if !ok {
			return state.Deployment{}, fmt.Errorf("unknown deployment %q", id)
		}
		return deployment, nil
	}
	deployment, ok := store.ActiveDeployment()
	if !ok {
		return state.Deployment{}, noActiveError(dir, store)
	}
	return deployment, nil
}

// resolveTarget loads the store and resolves id-or-active for one-shot
// verbs, so each verb repeats neither the load nor the lookup.
func resolveTarget(configPath, id string) (string, state.Store, state.Deployment, error) {
	dir := filepath.Dir(configPath)
	store, err := state.Load(dir)
	if err != nil {
		return "", state.Store{}, state.Deployment{}, err
	}
	deployment, err := resolveDeployment(dir, store, id)
	if err != nil {
		return "", state.Store{}, state.Deployment{}, err
	}
	return dir, store, deployment, nil
}

// isCharDevice reports whether f is a character device. Callers use it as
// a terminal approximation: every terminal is one, but so is /dev/null.
func isCharDevice(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// stdinInteractive reports whether prompts can reach a human. Overridable
// in tests; production checks for a character device.
var stdinInteractive = func(input io.Reader) bool {
	file, ok := input.(*os.File)
	return ok && isCharDevice(file)
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinner shows live state on terminals while long phases run. On
// non-terminals it stays silent and only prints the final line, so piped
// logs keep milestones (printed separately) without animation garbage.
type spinner struct {
	out     io.Writer
	message string
	frame   int
	animate bool
	stop    chan struct{}
	done    chan struct{}
	mu      sync.Mutex
}

func newSpinner(out io.Writer, message string) *spinner {
	spin := &spinner{
		out: out, message: message,
		stop: make(chan struct{}), done: make(chan struct{}),
	}
	if file, ok := out.(*os.File); ok && isCharDevice(file) {
		spin.animate = true
		go spin.spin()
	}
	return spin
}

// SetMessage updates the live state text.
func (spin *spinner) SetMessage(message string) {
	spin.mu.Lock()
	defer spin.mu.Unlock()
	spin.message = message
}

// render formats one animation frame. Pure for tests.
func (spin *spinner) render() string {
	spin.mu.Lock()
	defer spin.mu.Unlock()
	frame := spinnerFrames[spin.frame%len(spinnerFrames)]
	return "\r" + frame + " " + spin.message
}

func (spin *spinner) spin() {
	defer close(spin.done)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-spin.stop:
			return
		case <-ticker.C:
			spin.mu.Lock()
			spin.frame++
			spin.mu.Unlock()
			fmt.Fprint(spin.out, spin.render())
		}
	}
}

// Stop ends the animation and prints the outcome line.
func (spin *spinner) Stop(final string) {
	if spin.animate {
		close(spin.stop)
		<-spin.done
		fmt.Fprint(spin.out, "\r\033[K")
	}
	fmt.Fprintln(spin.out, final)
}

// readConfirmLine reads one prompt answer. It consumes exactly one line:
// a fresh buffered reader per call would swallow piped input past the
// newline, silently turning later answers into the default.
func readConfirmLine(input io.Reader) string {
	var line []byte
	one := make([]byte, 1)
	for {
		n, err := input.Read(one)
		if n > 0 {
			if one[0] == '\n' {
				break
			}
			line = append(line, one[0])
		}
		if err != nil {
			break
		}
	}
	return strings.ToLower(strings.TrimSpace(string(line)))
}

// tunnelSpec builds the SSH forward for a deployment. The tunnel always
// enforces the pinned host key; pinning happens before first contact.
func tunnelSpec(deployment state.Deployment) route.TunnelSpec {
	return route.TunnelSpec{
		SSHHost: deployment.SSH.Host, SSHPort: deployment.SSH.Port, SSHUser: deployment.SSH.User,
		IdentityFile: deployment.SSH.IdentityFile, KnownHostsFile: deployment.SSH.KnownHostsFile,
		LocalHost: deployment.Route.LocalHost, LocalPort: deployment.Route.LocalPort,
		RemoteHost: deployment.Route.RemoteHost, RemotePort: deployment.Route.RemotePort,
	}
}

// execTunnel is a started SSH forward process.
type execTunnel struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
	done   chan error
	once   sync.Once
	stop   error
}

// startExecTunnel builds and starts the forward. The caller supervises it.
func startExecTunnel(ctx context.Context, spec route.TunnelSpec, output io.Writer) (*execTunnel, error) {
	ctx, cancel := context.WithCancel(ctx)
	command, err := route.ForwardCommand(ctx, spec)
	if err != nil {
		cancel()
		return nil, err
	}
	command.Stdout, command.Stderr = output, output
	tunnel := &execTunnel{cmd: command, cancel: cancel, done: make(chan error, 1)}
	if err := command.Start(); err != nil {
		cancel()
		return nil, err
	}
	go func() { tunnel.done <- command.Wait() }()
	return tunnel, nil
}

// Start is a no-op: the process is already running.
func (tunnel *execTunnel) Start() error { return nil }

// Done reports the process exit.
func (tunnel *execTunnel) Done() <-chan error { return tunnel.done }

// Stop cancels the process and waits for its exit.
func (tunnel *execTunnel) Stop() error {
	tunnel.once.Do(func() {
		tunnel.cancel()
		tunnel.stop = <-tunnel.done
	})
	return tunnel.stop
}

// persistDeployment writes one deployment change back to the store file.
func persistDeployment(dir string, store *state.Store, deployment state.Deployment) error {
	if err := store.Update(deployment); err != nil {
		return err
	}
	return store.Save(dir)
}

// openTunnel starts one forward. Overridable in tests; production launches ssh.
var openTunnel = func(ctx context.Context, spec route.TunnelSpec, output io.Writer) (cli.Tunnel, error) {
	return startExecTunnel(ctx, spec, output)
}

// checkEndpoint verifies the local route answers. Overridable in tests.
var checkEndpoint = route.Healthcheck

// serveTunnel opens the forward for a prepared deployment, verifies the
// route, marks it tunneled, and supervises until exit. Afterwards the
// record settles to serving when the instance still runs, failed
// otherwise; cancellation maps to nil because the instance outlives us.
func serveTunnel(ctx context.Context, output io.Writer, dir string, store *state.Store, deployment state.Deployment, client *vast.Client) error {
	endpoint := fmt.Sprintf("http://%s:%d", deployment.Route.LocalHost, deployment.Route.LocalPort)
	tunnel, err := openTunnel(ctx, tunnelSpec(deployment), output)
	if err != nil {
		return err
	}
	healthCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	healthErr := checkEndpoint(healthCtx, endpoint)
	cancel()
	if healthErr != nil {
		_ = tunnel.Stop()
		return fmt.Errorf("server unhealthy at %s: %w", endpoint, healthErr)
	}
	deployment.State = state.Tunneled
	if err := store.SetActive(deployment.ID); err != nil {
		_ = tunnel.Stop()
		return err
	}
	if err := persistDeployment(dir, store, deployment); err != nil {
		_ = tunnel.Stop()
		return err
	}
	if _, err := fmt.Fprintf(output, "Tunnel ready at %s\n", endpoint); err != nil {
		_ = tunnel.Stop()
		return err
	}
	_, err = cli.SuperviseTunnel(ctx, output, tunnel, endpoint, cli.StartDependencies{
		Healthcheck: route.Healthcheck, Clock: setup.RealClock{},
		PollInterval: 5 * time.Second, PollTimeout: 30 * time.Minute,
	})
	refreshCtx, refreshCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer refreshCancel()
	instance, gerr := client.GetInstance(refreshCtx, deployment.Instance.ID)
	switch {
	case gerr != nil:
		// Unobservable after supervision end: keep Tunneled rather than
		// claim a failure we cannot see. The supervision error below
		// still signals the operator.
	case strings.EqualFold(instance.Status, "running"):
		deployment.State = state.Serving
		if serr := persistDeployment(dir, store, deployment); serr != nil {
			return serr
		}
	default:
		deployment.State = state.Failed
		if serr := persistDeployment(dir, store, deployment); serr != nil {
			return serr
		}
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
	return nil
}
