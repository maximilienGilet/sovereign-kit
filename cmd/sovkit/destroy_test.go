package main

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maximilienGilet/sovereign-kit/internal/state"
)

type vastHits struct {
	destroyInstance bool
	deleteKey       bool
}

func destroyFake(t *testing.T, hits *vastHits, exists bool, destroyOK bool, deleteOK bool) {
	t.Helper()
	fakeVast(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v0/instances/123456"):
			if exists {
				_, _ = w.Write([]byte(`{"instances":{"id":123456}}`))
			} else {
				_, _ = w.Write([]byte(`{"instances":null}`))
			}
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v0/instances/123456":
			hits.destroyInstance = true
			if destroyOK {
				_, _ = w.Write([]byte(`{"success":true}`))
			} else {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"success":false}`))
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v0/ssh/":
			_, _ = w.Write([]byte(`[{"id":7,"key":"ssh-ed25519 QUFBQQ== fixture"}]`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v0/ssh/7":
			hits.deleteKey = true
			if deleteOK {
				_, _ = w.Write([]byte(`{"success":true}`))
			} else {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"success":false}`))
			}
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
}

func TestDestroyHappyPathCleansEverything(t *testing.T) {
	dir := t.TempDir()
	writeLifecycleFixture(t, dir, "web-one", state.Serving)
	hits := &vastHits{}
	destroyFake(t, hits, true, true, true)
	var output bytes.Buffer
	if err := runWith([]string{"destroy", "--yes"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml")); err != nil {
		t.Fatal(err)
	}
	if !hits.destroyInstance || !hits.deleteKey {
		t.Fatalf("hits = %+v", hits)
	}
	deployment := loadDeployment(t, dir, "web-one")
	if deployment.State != state.Destroyed {
		t.Fatalf("state = %q", deployment.State)
	}
	store, err := state.Load(state.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	if store.Active != "" {
		t.Fatalf("active = %q, want cleared", store.Active)
	}
	entries, err := os.ReadDir(state.DeploymentDir(state.Dir(dir), "web-one"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("key files remain: %v", entries)
	}
}

func TestDestroyDeclineCancelsWithoutTouching(t *testing.T) {
	dir := t.TempDir()
	writeLifecycleFixture(t, dir, "web-one", state.Serving)
	previous := stdinInteractive
	stdinInteractive = func(io.Reader) bool { return true }
	t.Cleanup(func() { stdinInteractive = previous })
	hits := &vastHits{}
	destroyFake(t, hits, true, true, true)
	var output bytes.Buffer
	if err := runWith([]string{"destroy"}, strings.NewReader("n\n"), &output, filepath.Join(state.Dir(dir), "config.toml")); err != nil {
		t.Fatal(err)
	}
	if hits.destroyInstance || hits.deleteKey {
		t.Fatalf("hits = %+v", hits)
	}
	if deployment := loadDeployment(t, dir, "web-one"); deployment.State != state.Serving {
		t.Fatalf("state = %q, want unchanged", deployment.State)
	}
	if text := output.String(); !strings.Contains(text, "Cancelled") {
		t.Fatalf("output = %q", text)
	}
}

func TestDestroyRefusesWithoutTerminalOrYes(t *testing.T) {
	dir := t.TempDir()
	writeLifecycleFixture(t, dir, "web-one", state.Serving)
	hits := &vastHits{}
	destroyFake(t, hits, true, true, true)
	var output bytes.Buffer
	if err := runWith([]string{"destroy"}, strings.NewReader("y\n"), &output, filepath.Join(state.Dir(dir), "config.toml")); err == nil {
		t.Fatal("expected confirmation refusal")
	}
	if hits.destroyInstance {
		t.Fatal("must not destroy without confirmation")
	}
}

func TestDestroySkipsRemoteGoneInstance(t *testing.T) {
	dir := t.TempDir()
	writeLifecycleFixture(t, dir, "web-one", state.Failed)
	hits := &vastHits{}
	destroyFake(t, hits, false, true, true)
	var output bytes.Buffer
	if err := runWith([]string{"destroy", "--yes"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml")); err != nil {
		t.Fatal(err)
	}
	if hits.destroyInstance {
		t.Fatal("must not destroy an already-gone instance")
	}
	if deployment := loadDeployment(t, dir, "web-one"); deployment.State != state.Destroyed {
		t.Fatalf("state = %q", deployment.State)
	}
}

func TestDestroyMarksFailedWhenRemoteDestroyFails(t *testing.T) {
	dir := t.TempDir()
	writeLifecycleFixture(t, dir, "web-one", state.Serving)
	hits := &vastHits{}
	destroyFake(t, hits, true, false, true)
	var output bytes.Buffer
	err := runWith([]string{"destroy", "--yes"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml"))
	if err == nil || !strings.Contains(err.Error(), "may still bill") {
		t.Fatalf("expected billing warning, got %v", err)
	}
	if deployment := loadDeployment(t, dir, "web-one"); deployment.State != state.Failed {
		t.Fatalf("state = %q, want failed", deployment.State)
	}
}

func TestDestroyReportsKeyCleanupFailureButDestroys(t *testing.T) {
	dir := t.TempDir()
	writeLifecycleFixture(t, dir, "web-one", state.Serving)
	hits := &vastHits{}
	destroyFake(t, hits, true, true, false)
	var output bytes.Buffer
	err := runWith([]string{"destroy", "--yes"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml"))
	if err == nil || !strings.Contains(err.Error(), "cleanup failed") {
		t.Fatalf("expected key cleanup error, got %v", err)
	}
	if deployment := loadDeployment(t, dir, "web-one"); deployment.State != state.Destroyed {
		t.Fatalf("state = %q, want destroyed despite key failure", deployment.State)
	}
}

func TestDestroyRefusesAlreadyDestroyed(t *testing.T) {
	dir := t.TempDir()
	writeLifecycleFixture(t, dir, "web-one", state.Destroyed)
	hits := &vastHits{}
	destroyFake(t, hits, true, true, true)
	var output bytes.Buffer
	if err := runWith([]string{"destroy", "--yes"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml")); err == nil {
		t.Fatal("expected already-destroyed error")
	}
	if hits.destroyInstance || hits.deleteKey {
		t.Fatalf("hits = %+v", hits)
	}
}

func TestDestroySucceedsWhenKeyAlreadyAbsent(t *testing.T) {
	dir := t.TempDir()
	writeLifecycleFixture(t, dir, "web-one", state.Serving)
	fakeVast(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v0/instances/123456"):
			_, _ = w.Write([]byte(`{"instances":{"id":123456}}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v0/instances/123456":
			_, _ = w.Write([]byte(`{"success":true}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v0/ssh/":
			_, _ = w.Write([]byte(`[{"id":9,"key":"ssh-ed25519 QkJCQg== other"}]`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	var output bytes.Buffer
	if err := runWith([]string{"destroy", "--yes"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml")); err != nil {
		t.Fatalf("already-clean keys must succeed, got %v", err)
	}
	if text := output.String(); !strings.Contains(text, "already absent") {
		t.Fatalf("missing clean note:\n%s", text)
	}
	if deployment := loadDeployment(t, dir, "web-one"); deployment.State != state.Destroyed {
		t.Fatalf("state = %q", deployment.State)
	}
}

func TestDestroyFailsWhenIdentityGone(t *testing.T) {
	dir := t.TempDir()
	writeLifecycleFixture(t, dir, "web-one", state.Serving)
	sdir := state.Dir(dir)
	if err := os.Remove(state.IdentityPath(sdir, "web-one") + ".pub"); err != nil {
		t.Fatal(err)
	}
	hits := &vastHits{}
	destroyFake(t, hits, true, true, true)
	var output bytes.Buffer
	err := runWith([]string{"destroy", "--yes"}, strings.NewReader(""), &output, filepath.Join(sdir, "config.toml"))
	if err == nil || !strings.Contains(err.Error(), "cannot verify key cleanup") {
		t.Fatalf("expected unverifiable-hygiene error, got %v", err)
	}
	if deployment := loadDeployment(t, dir, "web-one"); deployment.State != state.Destroyed {
		t.Fatalf("state = %q, want destroyed despite hygiene error", deployment.State)
	}
}
