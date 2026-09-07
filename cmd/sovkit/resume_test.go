package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/cli"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/route"
	"github.com/maximilienGilet/sovereign-kit/internal/state"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

var (
	errTunnelRefused = errors.New("tunnel refused")
	errHostChanged   = errors.New("SSH host keys changed")
)

type resumeFake struct {
	statuses  []string
	calls     int
	started   bool
	host      string
	port      int
	failStart bool
}

func (fake *resumeFake) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v0/instances/123456" {
			status := "running"
			if fake.calls < len(fake.statuses) {
				status = fake.statuses[fake.calls]
			}
			fake.calls++
			_, _ = w.Write([]byte(`{"instances":{"id":123456,"actual_status":"` + status + `","ssh_host":"` + fake.host + `","ssh_port":` + strconv.Itoa(fake.port) + `}}`))
			return
		}
		if r.Method == http.MethodPut && r.URL.Path == "/api/v0/instances/123456/" {
			fake.started = true
			if fake.failStart {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"success":false}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})
}

func stubTrust(t *testing.T, old, next *config.SSH, err error) {
	t.Helper()
	previous := refreshHostTrust
	refreshHostTrust = func(_ context.Context, o, n config.SSH) error {
		*old, *next = o, n
		return err
	}
	t.Cleanup(func() { refreshHostTrust = previous })
}

func stubTunnelError(t *testing.T, err error) {
	t.Helper()
	previous := openTunnel
	openTunnel = func(context.Context, route.TunnelSpec, io.Writer) (cli.Tunnel, error) {
		return nil, err
	}
	t.Cleanup(func() { openTunnel = previous })
}

func TestResumeRefusesWrongStates(t *testing.T) {
	for _, st := range []state.DeploymentState{state.Tunneled, state.Destroyed, state.Planned, state.Renting} {
		dir := t.TempDir()
		writeLifecycleFixture(t, dir, "web-one", st)
		fakeVast(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}))
		var output bytes.Buffer
		if err := runWith([]string{"resume"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml")); err == nil {
			t.Fatalf("state %s: expected refusal", st)
		}
	}
}

func TestResumeRefusesWhenAnotherOwnsLive(t *testing.T) {
	dir := t.TempDir()
	writeLifecycleFixture(t, dir, "parked", state.Stopped)
	writeLifecycleFixture(t, dir, "live-one", state.Serving)
	fakeVast(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	var output bytes.Buffer
	err := runWith([]string{"resume", "parked"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml"))
	if err == nil || !strings.Contains(err.Error(), "live-one") {
		t.Fatalf("expected live-owner refusal, got %v", err)
	}
}

func TestResumeErrorsWhenInstanceGone(t *testing.T) {
	dir := t.TempDir()
	writeLifecycleFixture(t, dir, "web-one", state.Serving)
	fakeVast(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"instances":null}`))
	}))
	var output bytes.Buffer
	if err := runWith([]string{"resume"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml")); err == nil {
		t.Fatal("expected gone-instance error")
	}
}

func TestResumeStartsStoppedInstanceAndPinsTrust(t *testing.T) {
	dir := t.TempDir()
	writeLifecycleFixture(t, dir, "web-one", state.Stopped)
	fake := &resumeFake{statuses: []string{"stopped", "running"}, host: "new.example.test", port: 11111}
	fakeVast(t, fake.handler(t))
	var oldSSH, nextSSH config.SSH
	stubTrust(t, &oldSSH, &nextSSH, nil)
	stubTunnelError(t, errTunnelRefused)
	var output bytes.Buffer
	err := runWith([]string{"resume"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml"))
	if err == nil || !strings.Contains(err.Error(), "tunnel refused") {
		t.Fatalf("expected tunnel error, got %v", err)
	}
	if !fake.started {
		t.Fatal("start was not requested for stopped instance")
	}
	if nextSSH.Host != "new.example.test" || nextSSH.Port != 11111 {
		t.Fatalf("trust called with %+v", nextSSH)
	}
	deployment := loadDeployment(t, dir, "web-one")
	if deployment.SSH.Host != "new.example.test" || deployment.SSH.Port != 11111 {
		t.Fatalf("record not updated: %+v", deployment.SSH)
	}
	if deployment.State != state.Stopped {
		t.Fatalf("state = %q, want unchanged stopped", deployment.State)
	}
}

func TestResumeFailsOnChangedHostKey(t *testing.T) {
	dir := t.TempDir()
	writeLifecycleFixture(t, dir, "web-one", state.Serving)
	fake := &resumeFake{statuses: []string{"running"}, host: "h.example.test", port: 22022}
	fakeVast(t, fake.handler(t))
	var oldSSH, nextSSH config.SSH
	stubTrust(t, &oldSSH, &nextSSH, errHostChanged)
	var output bytes.Buffer
	if err := runWith([]string{"resume"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml")); err != errHostChanged {
		t.Fatalf("expected trust error, got %v", err)
	}
	if deployment := loadDeployment(t, dir, "web-one"); deployment.State != state.Serving {
		t.Fatalf("state = %q, want unchanged", deployment.State)
	}
}

func statusClient(t *testing.T, statuses []string, calls *int) *vast.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := "running"
		if *calls < len(statuses) {
			status = statuses[*calls]
		}
		*calls++
		_, _ = w.Write([]byte(`{"instances":{"id":123456,"actual_status":"` + status + `"}}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("VAST_API_KEY", "test-token")
	return vast.NewClient(server.URL, "test-token")
}
func TestWaitInstanceRunningDirect(t *testing.T) {
	calls := 0
	client := statusClient(t, []string{"running"}, &calls)
	if _, err := waitInstanceRunning(context.Background(), client, 123456, time.Minute); err != nil {
		t.Fatalf("immediate running: %v", err)
	}
	calls = 0
	client = statusClient(t, []string{"exited"}, &calls)
	if _, err := waitInstanceRunning(context.Background(), client, 123456, time.Minute); err == nil {
		t.Fatal("expected exited failure")
	}
	calls = 0
	client = statusClient(t, []string{"loading"}, &calls)
	if _, err := waitInstanceRunning(context.Background(), client, 123456, time.Nanosecond); err == nil {
		t.Fatal("expected timeout failure")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls = 0
	client = statusClient(t, []string{"loading"}, &calls)
	if _, err := waitInstanceRunning(ctx, client, 123456, time.Hour); err == nil {
		t.Fatal("expected cancellation failure")
	}
}
