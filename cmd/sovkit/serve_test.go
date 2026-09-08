package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/cli"
	"github.com/maximilienGilet/sovereign-kit/internal/route"
	"github.com/maximilienGilet/sovereign-kit/internal/state"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

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

func stubServeTunnel(t *testing.T, tunnel *gateTunnel) {
	t.Helper()
	previousOpen := openTunnel
	openTunnel = func(context.Context, route.TunnelSpec, io.Writer) (cli.Tunnel, error) {
		return tunnel, nil
	}
	t.Cleanup(func() { openTunnel = previousOpen })
}

// gateTunnel is a fake forward the test drives by hand.
type gateTunnel struct {
	done    chan error
	stopped bool
}

func (tunnel *gateTunnel) Start() error { return nil }

func (tunnel *gateTunnel) Done() <-chan error { return tunnel.done }

func (tunnel *gateTunnel) Stop() error {
	tunnel.stopped = true
	return nil
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

func TestServeTunnelEstablishFailureKeepsState(t *testing.T) {
	dir := t.TempDir()
	deployment := serveFixture(t, dir)
	done := make(chan error, 1)
	done <- errors.New("ssh exited")
	stubServeTunnel(t, &gateTunnel{done: done})
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
		t.Fatalf("state = %+v, want unchanged tunneled", persisted)
	}
}

func TestServeTunnelEstablishCancelledErrors(t *testing.T) {
	dir := t.TempDir()
	deployment := serveFixture(t, dir)
	done := make(chan error, 1)
	done <- errors.New("ssh exited")
	stubServeTunnel(t, &gateTunnel{done: done})
	store, err := state.Load(state.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer
	if err := serveTunnel(ctx, &output, state.Dir(dir), &store, deployment, serveClient(t, "running", false)); err == nil {
		t.Fatal("expected cancellation error during establishment")
	}
}

// liveEndpoint serves /v1/models on the fixture route. It skips when the
// port is occupied so the suite never flakes on a busy machine.
func liveEndpoint(t *testing.T) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:30000")
	if err != nil {
		t.Skipf("port 30000 occupied: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
}

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (buffer *safeBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buf.Write(data)
}

func (buffer *safeBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buf.String()
}

func waitOutput(t *testing.T, buffer *safeBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if strings.Contains(buffer.String(), want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("never saw %q in %q", want, buffer.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestServeTunnelHoldsRouteUntilCancel(t *testing.T) {
	liveEndpoint(t)
	dir := t.TempDir()
	deployment := serveFixture(t, dir)
	tunnel := &gateTunnel{done: make(chan error, 1)}
	stubServeTunnel(t, tunnel)
	store, err := state.Load(state.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	output := &safeBuffer{}
	result := make(chan error, 1)
	go func() {
		result <- serveTunnel(ctx, output, state.Dir(dir), &store, deployment, serveClient(t, "running", false))
	}()
	waitOutput(t, output, "Tunnel ready")
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("cancellation must map to nil, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not stop after cancel")
	}
	persisted, ok := mustLoad(t, dir, deployment.ID)
	if !ok || persisted.State != state.Serving {
		t.Fatalf("state = %+v, want serving", persisted)
	}
}

func TestServeTunnelCrashesToServingOnTunnelExit(t *testing.T) {
	liveEndpoint(t)
	dir := t.TempDir()
	deployment := serveFixture(t, dir)
	tunnel := &gateTunnel{done: make(chan error, 1)}
	stubServeTunnel(t, tunnel)
	store, err := state.Load(state.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output := &safeBuffer{}
	result := make(chan error, 1)
	go func() {
		result <- serveTunnel(ctx, output, state.Dir(dir), &store, deployment, serveClient(t, "running", false))
	}()
	waitOutput(t, output, "Tunnel ready")
	tunnel.done <- errors.New("ssh exited")
	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "ssh exited") {
			t.Fatalf("expected tunnel error, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not stop after tunnel exit")
	}
	persisted, ok := mustLoad(t, dir, deployment.ID)
	if !ok || persisted.State != state.Serving {
		t.Fatalf("state = %+v, want serving", persisted)
	}
}
