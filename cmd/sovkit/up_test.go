package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maximilienGilet/sovereign-kit/internal/cli"
	"github.com/maximilienGilet/sovereign-kit/internal/planner"
	"github.com/maximilienGilet/sovereign-kit/internal/provision"
	"github.com/maximilienGilet/sovereign-kit/internal/route"
	"github.com/maximilienGilet/sovereign-kit/internal/state"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

func upOffersFake(t *testing.T, rented *bool) {
	t.Helper()
	fakeVast(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v0/bundles":
			_, _ = w.Write([]byte(`{"offers":[{"id":7,"machine_id":9,"gpu_name":"RTX 5090","num_gpus":1,"gpu_ram":32607,"cpu_cores_effective":12,"cpu_ram":64000,"disk_space":200,"inet_down":1000,"inet_up":500,"driver_version":"570.124.06","dph_total":0.42,"geolocation":"FR","reliability":0.95},{"id":8,"machine_id":10,"gpu_name":"RTX 5090","num_gpus":1,"gpu_ram":32607,"cpu_cores_effective":12,"cpu_ram":64000,"disk_space":200,"inet_down":900,"inet_up":450,"driver_version":"570.124.06","dph_total":0.55,"geolocation":"DE","reliability":0.90}]}`))
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v0/asks/"):
			*rented = true
			_, _ = w.Write([]byte(`{"success":true,"new_contract":123456}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v0/instances/123456"):
			_, _ = w.Write([]byte(`{"instances":{"id":123456}}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Setenv("VAST_API_KEY", "test-token")
}

func TestUpDryRunPreviewsWithoutRenting(t *testing.T) {
	dir := t.TempDir()
	rented := false
	upOffersFake(t, &rented)
	var output bytes.Buffer
	if err := runWith([]string{"up", "qwen-solo-rtx5090", "--dry-run"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml")); err != nil {
		t.Fatal(err)
	}
	if rented {
		t.Fatal("dry run must not rent")
	}
	text := output.String()
	for _, want := range []string{"Dry run", "qwen-solo-rtx5090", "Would rent", "0.42"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q:\n%s", want, text)
		}
	}
	if _, err := state.Load(state.Dir(dir)); err != nil {
		t.Fatal(err)
	} else {
		store, _ := state.Load(state.Dir(dir))
		if len(store.Deployments) != 0 {
			t.Fatal("dry run must not persist records")
		}
	}
}

func TestUpDryRunHonorsOfferFlag(t *testing.T) {
	dir := t.TempDir()
	rented := false
	upOffersFake(t, &rented)
	var output bytes.Buffer
	if err := runWith([]string{"up", "qwen-solo-rtx5090", "--dry-run", "--offer", "8"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml")); err != nil {
		t.Fatal(err)
	}
	if text := output.String(); !strings.Contains(text, "offer #8") {
		t.Fatalf("expected offer 8 preview:\n%s", text)
	}
}

func TestUpRejectsVerifyWithYes(t *testing.T) {
	dir := t.TempDir()
	var output bytes.Buffer
	err := runWith([]string{"up", "qwen-solo-rtx5090", "--verify-host-key", "--yes"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml"))
	var usage *usageError
	if !errors.As(err, &usage) {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestUpRejectsUnknownOffer(t *testing.T) {
	dir := t.TempDir()
	rented := false
	upOffersFake(t, &rented)
	var output bytes.Buffer
	err := runWith([]string{"up", "qwen-solo-rtx5090", "--dry-run", "--offer", "999"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml"))
	if err == nil || !strings.Contains(err.Error(), "not eligible") {
		t.Fatalf("expected ineligible error, got %v", err)
	}
}

func TestUpRefusesLiveDeployment(t *testing.T) {
	dir := t.TempDir()
	writeLifecycleFixture(t, dir, "live-one", state.Serving)
	rented := false
	upOffersFake(t, &rented)
	var output bytes.Buffer
	err := runWith([]string{"up", "qwen-solo-rtx5090", "--yes"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml"))
	if err == nil || !strings.Contains(err.Error(), "live-one") {
		t.Fatalf("expected live refusal, got %v", err)
	}
	if rented {
		t.Fatal("must not rent while another owns a live instance")
	}
}

func TestUpRefusesAmbiguousNonInteractive(t *testing.T) {
	dir := t.TempDir()
	rented := false
	upOffersFake(t, &rented)
	var output bytes.Buffer
	err := runWith([]string{"up", "qwen-solo-rtx5090"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml"))
	if err == nil || !strings.Contains(err.Error(), "multiple offers match") {
		t.Fatalf("expected ambiguity refusal, got %v", err)
	}
}

func TestChooseOfferCoversModes(t *testing.T) {
	recs := func() []planner.Recommendation {
		return []planner.Recommendation{
			{Offer: vast.Offer{ID: 7, HourlyUSD: 0.42}},
			{Offer: vast.Offer{ID: 8, HourlyUSD: 0.55}},
		}
	}
	if chosen, err := chooseOffer(strings.NewReader(""), &bytes.Buffer{}, recs(), 8, false); err != nil || chosen.Offer.ID != 8 {
		t.Fatalf("flag = %+v, %v", chosen, err)
	}
	if _, err := chooseOffer(strings.NewReader(""), &bytes.Buffer{}, recs(), 999, false); err == nil {
		t.Fatal("expected unknown offer error")
	}
	if chosen, err := chooseOffer(strings.NewReader(""), &bytes.Buffer{}, recs(), 0, true); err != nil || chosen.Offer.ID != 7 {
		t.Fatalf("yes = %+v, %v", chosen, err)
	}
	if chosen, err := chooseOffer(strings.NewReader(""), &bytes.Buffer{}, recs()[:1], 0, false); err != nil || chosen.Offer.ID != 7 {
		t.Fatalf("single = %+v, %v", chosen, err)
	}
	previous := stdinInteractive
	stdinInteractive = func(io.Reader) bool { return true }
	t.Cleanup(func() { stdinInteractive = previous })
	var output bytes.Buffer
	if chosen, err := chooseOffer(strings.NewReader("2\n"), &output, recs(), 0, false); err != nil || chosen.Offer.ID != 8 {
		t.Fatalf("prompt pick = %+v, %v", chosen, err)
	}
	if chosen, err := chooseOffer(strings.NewReader("\n"), &output, recs(), 0, false); err != nil || chosen.Offer.ID != 7 {
		t.Fatalf("prompt default = %+v, %v", chosen, err)
	}
	if _, err := chooseOffer(strings.NewReader("x\nx\nx\n"), &output, recs(), 0, false); err == nil {
		t.Fatal("expected invalid selection error")
	}
}

func TestUpWiresProvisionInputs(t *testing.T) {
	dir := t.TempDir()
	rented := false
	upOffersFake(t, &rented)
	previousProvision := provisionPrepare
	var captured provision.Inputs
	provisionPrepare = func(_ context.Context, store *state.Store, dir string, inputs provision.Inputs, _ provision.Deps) (state.Deployment, error) {
		captured = inputs
		deployment := state.Deployment{
			ID: inputs.DeploymentID, RecipeID: inputs.Recipe.ID, RecipeVersion: inputs.Recipe.Version,
			State: state.Preparing,
			SSH: state.SSH{
				Host: "h.example.test", Port: 22022, User: "root",
				IdentityFile:   state.IdentityPath(dir, inputs.DeploymentID),
				KnownHostsFile: state.KnownHostsPath(dir, inputs.DeploymentID),
			},
			Route: state.Route{LocalHost: "127.0.0.1", LocalPort: inputs.Port, RemoteHost: "127.0.0.1", RemotePort: 30000},
		}
		if err := store.Add(deployment); err != nil {
			return state.Deployment{}, err
		}
		store.SetActive(deployment.ID)
		if err := store.Save(dir); err != nil {
			return state.Deployment{}, err
		}
		return deployment, nil
	}
	t.Cleanup(func() { provisionPrepare = previousProvision })
	previousTunnel := openTunnel
	openTunnel = func(context.Context, route.TunnelSpec, io.Writer) (cli.Tunnel, error) {
		return nil, errTunnelRefused
	}
	t.Cleanup(func() { openTunnel = previousTunnel })
	var output bytes.Buffer
	err := runWith([]string{"up", "qwen-solo-rtx5090", "--yes", "--cap", "1"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml"))
	if err == nil || !strings.Contains(err.Error(), "tunnel refused") {
		t.Fatalf("expected tunnel error, got %v", err)
	}
	if captured.Offer.ID != 7 || captured.Port != 30000 || captured.CapUSD != 1 {
		t.Fatalf("inputs = %+v", captured)
	}
	if captured.Recipe.ID != "qwen-solo-rtx5090" || captured.Recipe.Runtime.Image == "" {
		t.Fatalf("recipe = %+v", captured.Recipe)
	}
	deployment, ok := mustLoad(t, dir, captured.DeploymentID)
	if !ok || deployment.State != state.Preparing {
		t.Fatalf("record = %+v", deployment)
	}
}
func mustLoad(t *testing.T, dir, id string) (state.Deployment, bool) {
	t.Helper()
	store, err := state.Load(state.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	deployment, ok := store.Get(id)
	return deployment, ok
}

func TestUpCleansGoneFailedDeploymentAndProceeds(t *testing.T) {
	dir := t.TempDir()
	writeLifecycleFixture(t, dir, "old-failed", state.Failed)
	fakeVast(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v0/bundles":
			_, _ = w.Write([]byte(`{"offers":[{"id":7,"machine_id":9,"gpu_name":"RTX 5090","num_gpus":1,"gpu_ram":32607,"cpu_cores_effective":12,"cpu_ram":64000,"disk_space":200,"inet_down":1000,"inet_up":500,"driver_version":"570.124.06","dph_total":0.42,"geolocation":"FR","reliability":0.95}]}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v0/instances/123456"):
			_, _ = w.Write([]byte(`{"instances":null}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v0/ssh/":
			_, _ = w.Write([]byte(`[{"id":7,"key":"ssh-ed25519 QUFBQQ== fixture"}]`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v0/ssh/7":
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Setenv("VAST_API_KEY", "test-token")
	previousProvision := provisionPrepare
	provisionPrepare = func(_ context.Context, store *state.Store, dir string, inputs provision.Inputs, _ provision.Deps) (state.Deployment, error) {
		deployment := state.Deployment{
			ID: inputs.DeploymentID, RecipeID: inputs.Recipe.ID, RecipeVersion: inputs.Recipe.Version,
			State: state.Preparing,
			SSH: state.SSH{
				Host: "h", Port: 22, User: "root",
				IdentityFile:   state.IdentityPath(dir, inputs.DeploymentID),
				KnownHostsFile: state.KnownHostsPath(dir, inputs.DeploymentID),
			},
			Route: state.Route{LocalHost: "127.0.0.1", LocalPort: inputs.Port, RemoteHost: "127.0.0.1", RemotePort: 30000},
		}
		if err := store.Add(deployment); err != nil {
			return state.Deployment{}, err
		}
		store.SetActive(deployment.ID)
		if err := store.Save(dir); err != nil {
			return state.Deployment{}, err
		}
		return deployment, nil
	}
	t.Cleanup(func() { provisionPrepare = previousProvision })
	previousTunnel := openTunnel
	openTunnel = func(context.Context, route.TunnelSpec, io.Writer) (cli.Tunnel, error) {
		return nil, errTunnelRefused
	}
	t.Cleanup(func() { openTunnel = previousTunnel })
	var output bytes.Buffer
	err := runWith([]string{"up", "qwen-solo-rtx5090", "--yes"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml"))
	if err == nil || !strings.Contains(err.Error(), "tunnel refused") {
		t.Fatalf("expected to proceed past cleanup, got %v", err)
	}
	if text := output.String(); !strings.Contains(text, "cleaning up") {
		t.Fatalf("missing cleanup note:\n%s", text)
	}
	old, ok := mustLoad(t, dir, "old-failed")
	if !ok || old.State != state.Destroyed {
		t.Fatalf("old record = %+v", old)
	}
}
func TestUpBlocksLiveFailedDeployment(t *testing.T) {
	dir := t.TempDir()
	writeLifecycleFixture(t, dir, "old-failed", state.Failed)
	fakeVast(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v0/bundles" {
			_, _ = w.Write([]byte(`{"offers":[{"id":7,"machine_id":9,"gpu_name":"RTX 5090","num_gpus":1,"gpu_ram":32607,"cpu_cores_effective":12,"cpu_ram":64000,"disk_space":200,"inet_down":1000,"inet_up":500,"driver_version":"570.124.06","dph_total":0.42,"geolocation":"FR","reliability":0.95}]}`))
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v0/instances/123456") {
			_, _ = w.Write([]byte(`{"instances":{"id":123456}}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	t.Setenv("VAST_API_KEY", "test-token")
	var output bytes.Buffer
	err := runWith([]string{"up", "qwen-solo-rtx5090", "--yes"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml"))
	if err == nil || !strings.Contains(err.Error(), "may still bill") {
		t.Fatalf("expected live block, got %v", err)
	}
}

func TestUpBlocksWhenVerificationFails(t *testing.T) {
	dir := t.TempDir()
	writeLifecycleFixture(t, dir, "old-failed", state.Failed)
	fakeVast(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v0/bundles" {
			_, _ = w.Write([]byte(`{"offers":[{"id":7,"machine_id":9,"gpu_name":"RTX 5090","num_gpus":1,"gpu_ram":32607,"cpu_cores_effective":12,"cpu_ram":64000,"disk_space":200,"inet_down":1000,"inet_up":500,"driver_version":"570.124.06","dph_total":0.42,"geolocation":"FR","reliability":0.95}]}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Setenv("VAST_API_KEY", "test-token")
	var output bytes.Buffer
	err := runWith([]string{"up", "qwen-solo-rtx5090", "--yes"}, strings.NewReader(""), &output, filepath.Join(state.Dir(dir), "config.toml"))
	if err == nil || !strings.Contains(err.Error(), "cannot verify") {
		t.Fatalf("expected verification block, got %v", err)
	}
}
