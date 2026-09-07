package main

import (
	"bytes"
	"context"
	"errors"
	"github.com/maximilienGilet/sovereign-kit/internal/cli"
	"github.com/maximilienGilet/sovereign-kit/internal/route"
	"github.com/maximilienGilet/sovereign-kit/internal/state"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type stubTunnel struct {
	done chan error
}

func (tunnel *stubTunnel) Start() error { return nil }

func (tunnel *stubTunnel) Done() <-chan error { return tunnel.done }

func (tunnel *stubTunnel) Stop() error { return nil }

func serveFixture(t *testing.T, dir string) state.Deployment {
	t.Helper()
	sdir := state.Dir(dir)
	store, err := state.Load(sdir)
	if err != nil {
		t.Fatal(err)
	}
	deployment := state.Deployment{
		ID: "web-one", RecipeID: "qwen-solo-rtx5090", RecipeVersion: 1,
		Instance: state.Instance{ID: 123456, Status: "running"},
		SSH:      state.SSH{Host: "h", Port: 22, User: "root", IdentityFile: "i", KnownHostsFile: "k"},
		Route:    state.Route{LocalHost: "127.0.0.1", LocalPort: 30000, RemoteHost: "127.0.0.1", RemotePort: 30000},
		State:    state.Tunneled,
	}
	if err := store.Add(deployment); err != nil {
		t.Fatal(err)
	}
	store.SetActive(deployment.ID)
	if err := store.Save(sdir); err != nil {
		t.Fatal(err)
	}
	return deployment
}

func stubServeTunnel(t *testing.T, tunnelErr error, healthErr error) {
	t.Helper()
	previousOpen := openTunnel
	done := make(chan error, 1)
	done <- tunnelErr
	openTunnel = func(context.Context, route.TunnelSpec, io.Writer) (cli.Tunnel, error) {
		return &stubTunnel{done: done}, nil
	}
	t.Cleanup(func() { openTunnel = previousOpen })
	previousCheck := checkEndpoint
	checkEndpoint = func(context.Context, string) error { return healthErr }
	t.Cleanup(func() { checkEndpoint = previousCheck })
}

func serveClient(t *testing.T, status string, fail bool) *vast.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"instances":{"id":123456,"actual_status":"` + status + `"}}`))
	}))
	t.Cleanup(server.Close)
	return vast.NewClient(server.URL, "test-token")
}

func TestServeTunnelKeepsTunneledWhenStatusUnobservable(t *testing.T) {
	dir := t.TempDir()
	deployment := serveFixture(t, dir)
	stubServeTunnel(t, errors.New("ssh exited"), nil)
	store, err := state.Load(state.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err = serveTunnel(context.Background(), &output, state.Dir(dir), &store, deployment, serveClient(t, "", true))
	if err == nil || !strings.Contains(err.Error(), "ssh exited") {
		t.Fatalf("expected tunnel error, got %v", err)
	}
	persisted, ok := mustLoad(t, dir, deployment.ID)
	if !ok || persisted.State != state.Tunneled {
		t.Fatalf("state = %+v, want kept tunneled", persisted)
	}
}

func TestServeTunnelSettlesServingWhenRunning(t *testing.T) {
	dir := t.TempDir()
	deployment := serveFixture(t, dir)
	stubServeTunnel(t, errors.New("ssh exited"), nil)
	store, err := state.Load(state.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err = serveTunnel(context.Background(), &output, state.Dir(dir), &store, deployment, serveClient(t, "running", false))
	if err == nil || !strings.Contains(err.Error(), "ssh exited") {
		t.Fatalf("expected tunnel error, got %v", err)
	}
	persisted, ok := mustLoad(t, dir, deployment.ID)
	if !ok || persisted.State != state.Serving {
		t.Fatalf("state = %+v, want serving", persisted)
	}
}

func TestServeTunnelMapsCancellationToNil(t *testing.T) {
	dir := t.TempDir()
	deployment := serveFixture(t, dir)
	stubServeTunnel(t, context.Canceled, nil)
	store, err := state.Load(state.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := serveTunnel(context.Background(), &output, state.Dir(dir), &store, deployment, serveClient(t, "running", false)); err != nil {
		t.Fatalf("cancellation must map to nil, got %v", err)
	}
	persisted, ok := mustLoad(t, dir, deployment.ID)
	if !ok || persisted.State != state.Serving {
		t.Fatalf("state = %+v, want serving", persisted)
	}
}
