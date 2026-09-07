package main

import (
	"bytes"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maximilienGilet/sovereign-kit/internal/state"
)

func TestLogsRequiresDeploymentAndToken(t *testing.T) {
	dir := t.TempDir()
	var output bytes.Buffer
	if err := runWith([]string{"logs"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml")); err == nil {
		t.Fatal("expected no-active error")
	}
	writeLifecycleFixture(t, dir, "web-one", state.Serving)
	t.Setenv("VAST_API_KEY", "")
	if err := runWith([]string{"logs"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml")); err == nil {
		t.Fatal("expected token error")
	}
}

func TestLogsSurfacesAPIError(t *testing.T) {
	dir := t.TempDir()
	writeLifecycleFixture(t, dir, "web-one", state.Serving)
	fakeVast(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v0/instances/request_logs/") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	var output bytes.Buffer
	if err := runWith([]string{"logs"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml")); err == nil {
		t.Fatal("expected logs error")
	}
}
