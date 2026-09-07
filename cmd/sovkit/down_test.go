package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/state"
)

func fakeVast(t *testing.T, handler http.Handler) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	previous := vastAPIBaseURL
	vastAPIBaseURL = server.URL
	t.Cleanup(func() { vastAPIBaseURL = previous })
	t.Setenv("VAST_API_KEY", "test-token")
}

func writeLifecycleFixture(t *testing.T, dir, id string, st state.DeploymentState) state.Deployment {
	t.Helper()
	sdir := state.Dir(dir)
	store, err := state.Load(sdir)
	if err != nil {
		t.Fatal(err)
	}
	deployment := state.Deployment{
		ID: id, RecipeID: "qwen-solo-rtx5090", RecipeVersion: 1,
		Pins: state.Pins{
			ImageDigest:     "ghcr.io/example/llama@sha256:a9ceb65277f2f4dfd4cc3b5896f4398703e323249dd9bf87eb20b17b9f467239",
			ModelRepository: "unsloth/Qwen3.8-27B-GGUF", ModelRevision: "4ca720788d1e01f1bff70c033e0d0028fd02e502",
			ModelFilename: "Qwen3.8-27B-UD-Q4_K_XL.gguf", ModelSHA256: "3f227079003add2511437e5b1e94812e363385225bf6a9b47b0054a72bc8b01e",
		},
		Instance: state.Instance{
			ID: 123456, Status: "running",
			Offer: state.OfferSnapshot{ID: 7, GPUName: "RTX 5090", GPUCount: 1, GPUVRAMGB: 32.607, HourlyUSD: 0.42, Location: "FR"},
			Image: "ghcr.io/example/llama@sha256:a9ceb65277f2f4dfd4cc3b5896f4398703e323249dd9bf87eb20b17b9f467239",
		},
		SSH: state.SSH{
			Host: "h.example.test", Port: 22022, User: "root",
			IdentityFile: state.IdentityPath(sdir, id), KnownHostsFile: state.KnownHostsPath(sdir, id),
		},
		Route:     state.Route{LocalHost: "127.0.0.1", LocalPort: 30000, RemoteHost: "127.0.0.1", RemotePort: 30000},
		Spend:     state.Spend{HourlyUSD: 0.42, TotalUSD: 1.26},
		State:     st,
		CreatedAt: time.Now().UTC().Add(-90 * time.Minute),
		UpdatedAt: time.Now().UTC().Add(-90 * time.Minute),
	}
	if err := store.Add(deployment); err != nil {
		t.Fatal(err)
	}
	if err := store.SetActive(id); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(sdir); err != nil {
		t.Fatal(err)
	}
	deployDir := state.DeploymentDir(sdir, id)
	if err := os.MkdirAll(deployDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state.IdentityPath(sdir, id), []byte("PRIVATE"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state.IdentityPath(sdir, id)+".pub", []byte("ssh-ed25519 QUFBQQ== fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state.KnownHostsPath(sdir, id), []byte("h.example.test ssh-ed25519 QUFBQQ=="), 0o600); err != nil {
		t.Fatal(err)
	}
	return deployment
}

func loadDeployment(t *testing.T, dir, id string) state.Deployment {
	t.Helper()
	store, err := state.Load(state.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	deployment, ok := store.Get(id)
	if !ok {
		t.Fatalf("deployment %q missing", id)
	}
	return deployment
}

func TestDownStopsLiveInstance(t *testing.T) {
	dir := t.TempDir()
	writeLifecycleFixture(t, dir, "web-one", state.Serving)
	var stopped bool
	fakeVast(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v0/instances/123456/" {
			stopped = true
			body := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(body)
			if !strings.Contains(string(body), `"stopped"`) {
				t.Errorf("stop body = %s", body)
			}
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	var output bytes.Buffer
	if err := runWith([]string{"down"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml")); err != nil {
		t.Fatal(err)
	}
	if !stopped {
		t.Fatal("stop was not requested")
	}
	deployment := loadDeployment(t, dir, "web-one")
	if deployment.State != state.Stopped {
		t.Fatalf("state = %q", deployment.State)
	}
	store, err := state.Load(state.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	if store.Active != "" {
		t.Fatalf("active = %q, want cleared", store.Active)
	}
	if text := output.String(); !strings.Contains(text, "billing paused") {
		t.Fatalf("output = %q", text)
	}
}

func TestDownIsIdempotentOnStopped(t *testing.T) {
	dir := t.TempDir()
	writeLifecycleFixture(t, dir, "web-one", state.Stopped)
	fakeVast(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	var output bytes.Buffer
	if err := runWith([]string{"down", "web-one"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml")); err != nil {
		t.Fatal(err)
	}
	if text := output.String(); !strings.Contains(text, "already stopped") {
		t.Fatalf("output = %q", text)
	}
}

func TestDownRefusesDestroyedAndMissing(t *testing.T) {
	dir := t.TempDir()
	writeLifecycleFixture(t, dir, "web-one", state.Destroyed)
	fakeVast(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	var output bytes.Buffer
	if err := runWith([]string{"down", "web-one"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml")); err == nil {
		t.Fatal("expected already-destroyed error")
	}
	if err := runWith([]string{"down", "missing"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml")); err == nil {
		t.Fatal("expected unknown deployment error")
	}
}

func TestDownKeepsStateOnAPIFailure(t *testing.T) {
	dir := t.TempDir()
	writeLifecycleFixture(t, dir, "web-one", state.Serving)
	fakeVast(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"success":false}`))
	}))
	var output bytes.Buffer
	if err := runWith([]string{"down"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml")); err == nil {
		t.Fatal("expected stop error")
	}
	if deployment := loadDeployment(t, dir, "web-one"); deployment.State != state.Serving {
		t.Fatalf("state = %q, want unchanged serving", deployment.State)
	}
}
