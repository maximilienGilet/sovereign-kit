package cli

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
)

// Every edge is an in-memory delayed fixture. This never creates, connects to,
// or destroys a provider resource. The root remains the sole Tea program.
func TestPreviewProvisioningPTY(t *testing.T) {
	if os.Getenv("SOVKIT_PREVIEW_PROVISIONING") != "1" {
		t.Skip("opt-in fake-provider PTY preview")
	}
	m := newApplication(context.Background(), t.TempDir()+"/config", "root", "home", ApplicationDependencies{})
	ctx, cancel := context.WithCancel(context.Background())
	s := &applicationSession{ctx: ctx, cancel: cancel, generation: 1, events: make(chan any, 16), done: make(chan struct{})}
	m.generation = 1
	m.session = s
	m.screen = "working"
	m.status = "Creating instance…"
	p := &applicationPrompter{session: s}
	delay := func(ctx context.Context, d time.Duration) error {
		select {
		case <-time.After(d):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	go func() {
		defer close(s.done)
		p.SetupProgress(setup.Progress{Stage: setup.ProgressCreating})
		if s.err = delay(ctx, time.Second); s.err != nil {
			return
		}
		p.InstanceCreated(setup.InstanceRecovery{InstanceID: 73142, Destroy: func(cleanup context.Context) error { return delay(cleanup, 3*time.Second) }})
		p.SetupProgress(setup.Progress{Stage: setup.ProgressWaiting, InstanceID: 73142})
		for _, message := range []string{"7b9dbe46b1aa: Pull complete", "a7eed62228ee: Verifying Checksum", "a7eed62228ee: Download complete", "9b18e2ebedf4: Verifying Checksum", "9b18e2ebedf4: Pull complete"} {
			p.InstanceActivity(setup.Activity{InstanceID: 73142, Status: "loading", Message: message, CheckedAt: time.Now()})
			if s.err = delay(ctx, 2*time.Second); s.err != nil {
				return
			}
		}
		p.SetupProgress(setup.Progress{Stage: setup.ProgressHostKeys, InstanceID: 73142})
		if s.err = delay(ctx, 2*time.Second); s.err != nil {
			return
		}
		p.SetupProgress(setup.Progress{Stage: setup.ProgressLaunching, InstanceID: 73142})
		if s.err = delay(ctx, 3*time.Second); s.err != nil {
			return
		}
		s.err = errors.New("Fixture: inference server did not start. No real instance exists.")
	}()
	defer m.cleanup()
	_, err := tea.NewProgram(previewProvisioningRoot{m}, tea.WithInput(os.Stdin), tea.WithOutput(os.Stdout), tea.WithAltScreen()).Run()
	if err != nil {
		t.Fatal(err)
	}
}

type previewProvisioningRoot struct{ *applicationModel }

func (m previewProvisioningRoot) Init() tea.Cmd { return m.session.next() }
