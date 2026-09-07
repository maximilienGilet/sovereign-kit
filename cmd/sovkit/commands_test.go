package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maximilienGilet/sovereign-kit/internal/state"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

func TestRunHelpListsTheReadVerbs(t *testing.T) {
	var output bytes.Buffer
	if err := run([]string{"help"}, &output); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"recipes", "offers", "status", "doctor"} {
		if !strings.Contains(output.String(), command) {
			t.Fatalf("help does not describe %q: %s", command, output.String())
		}
	}
	for _, removed := range []string{"dashboard", "catalog", "setup", "resume", "start", "tunnel"} {
		if strings.Contains(output.String(), removed) {
			t.Fatalf("help still describes removed %q: %s", removed, output.String())
		}
	}
}

func TestBareArgsShowUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"-h"}} {
		var output bytes.Buffer
		if err := runWith(args, &output, "missing"); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), "Usage:") {
			t.Fatalf("missing usage: %s", output.String())
		}
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	for _, args := range [][]string{{"unknown"}, {"dashboard"}, {"catalog"}} {
		var output bytes.Buffer
		err := run(args, &output)
		if err == nil {
			t.Fatalf("expected an error for %q", args[0])
		}
		var usage *usageError
		if !errors.As(err, &usage) {
			t.Fatalf("expected usage error for %q, got %T", args[0], err)
		}
	}
}

func TestExitCodeMapsErrors(t *testing.T) {
	if exitCode(nil) != 0 {
		t.Fatal("nil must exit 0")
	}
	if exitCode(usageErrorf("bad")) != 2 {
		t.Fatal("usage must exit 2")
	}
	if exitCode(errors.New("boom")) != 1 {
		t.Fatal("operational must exit 1")
	}
	if exitCode(&doctorExitError{code: 3}) != 3 {
		t.Fatal("doctor must keep its code")
	}
}

func TestFilterOffersByCapKeepsCheapAndKnown(t *testing.T) {
	offers := []vast.Offer{
		{HourlyUSD: 0.42},
		{HourlyUSD: 0.50},
		{HourlyUSD: 0.51},
		{PriceUnknown: true, HourlyUSD: 0.10},
	}
	kept := filterOffersByCap(offers, 0.50)
	if len(kept) != 2 || kept[0].HourlyUSD != 0.42 || kept[1].HourlyUSD != 0.50 {
		t.Fatalf("kept = %+v", kept)
	}
	if got := filterOffersByCap(offers, 0); len(got) != len(offers) {
		t.Fatalf("no cap must pass through, got %d", len(got))
	}
}

func TestParseFlagsAroundPositionalAcceptsFlagsEitherSide(t *testing.T) {
	newSet := func() (*flag.FlagSet, *bool, *int) {
		set := flag.NewFlagSet("test", flag.ContinueOnError)
		set.SetOutput(io.Discard)
		asJSON := set.Bool("json", false, "")
		limit := set.Int("limit", 20, "")
		return set, asJSON, limit
	}
	set, asJSON, limit := newSet()
	positionals, err := parseFlagsAroundPositional(set, []string{"qwen-solo", "--json", "--limit", "5"})
	if err != nil {
		t.Fatal(err)
	}
	if len(positionals) != 1 || positionals[0] != "qwen-solo" || !*asJSON || *limit != 5 {
		t.Fatalf("recipe-first: %q json=%v limit=%d", positionals, *asJSON, *limit)
	}
	set, asJSON, limit = newSet()
	positionals, err = parseFlagsAroundPositional(set, []string{"--json", "qwen-solo", "--limit", "5"})
	if err != nil {
		t.Fatal(err)
	}
	if len(positionals) != 1 || positionals[0] != "qwen-solo" || !*asJSON || *limit != 5 {
		t.Fatalf("flag-first: %q json=%v limit=%d", positionals, *asJSON, *limit)
	}
	set, _, _ = newSet()
	if _, err := parseFlagsAroundPositional(set, []string{"--bogus"}); err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestCountryListAcceptsRepeatAndCommaValues(t *testing.T) {
	var list countryList
	if err := list.Set("FR"); err != nil {
		t.Fatal(err)
	}
	if err := list.Set("DE, US"); err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 || list[0] != "FR" || list[1] != "DE" || list[2] != "US" {
		t.Fatalf("list = %q", list)
	}
}

func TestResolveGPUModelRefusesStrictOverride(t *testing.T) {
	resolved, err := builtinRecipe("qwen-solo-rtx5090")
	if err != nil {
		t.Fatal(err)
	}
	model, strict, err := resolveGPUModel(resolved, "")
	if err != nil || model != "RTX 5090" || !strict {
		t.Fatalf("defaults = %q %v %v", model, strict, err)
	}
	if _, _, err := resolveGPUModel(resolved, "RTX 4090"); err == nil {
		t.Fatal("expected strict override refusal")
	} else {
		var usage *usageError
		if !errors.As(err, &usage) {
			t.Fatalf("expected usage error, got %T", err)
		}
	}
	if model, strict, err := resolveGPUModel(resolved, "RTX 5090"); err != nil || model != "RTX 5090" || !strict {
		t.Fatalf("same-model flag = %q %v %v", model, strict, err)
	}
}

func TestResolveInterruptibleEnforcesRecipeGate(t *testing.T) {
	resolved, err := builtinRecipe("qwen-solo-rtx5090")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolveInterruptible(resolved, true); err == nil {
		t.Fatal("expected refusal for forbidding recipe")
	}
	if bid, err := resolveInterruptible(resolved, false); err != nil || bid {
		t.Fatalf("no-flag = %v %v", bid, err)
	}
}

func TestRecipesListsBuiltin(t *testing.T) {
	var output bytes.Buffer
	if err := runWith([]string{"recipes"}, &output, "missing"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"qwen-studio", "qwen-solo-rtx5090"} {
		if !strings.Contains(output.String(), id) {
			t.Fatalf("missing recipe %q:\n%s", id, output.String())
		}
	}
}

func TestRecipesJSONParses(t *testing.T) {
	var output bytes.Buffer
	if err := runWith([]string{"recipes", "--json"}, &output, "missing"); err != nil {
		t.Fatal(err)
	}
	var list []map[string]any
	if err := json.Unmarshal(output.Bytes(), &list); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, output.String())
	}
	if len(list) != 5 {
		t.Fatalf("recipes = %d, want 5", len(list))
	}
}

func TestOffersRejectsUnknownRecipeWithoutNetwork(t *testing.T) {
	var output bytes.Buffer
	err := runWith([]string{"offers", "nope"}, &output, "missing")
	var usage *usageError
	if !errors.As(err, &usage) {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestOffersRefusesInterruptibleForForbiddingRecipe(t *testing.T) {
	var output bytes.Buffer
	err := runWith([]string{"offers", "qwen-solo-rtx5090", "--interruptible"}, &output, "missing")
	if err == nil || !strings.Contains(err.Error(), "forbids interruptible") {
		t.Fatalf("expected gate refusal, got %v", err)
	}
	var usage *usageError
	if !errors.As(err, &usage) {
		t.Fatalf("expected usage error, got %T", err)
	}
}

func TestOffersRequiresToken(t *testing.T) {
	t.Setenv("VAST_API_KEY", "")
	dir := t.TempDir()
	var output bytes.Buffer
	err := runWith([]string{"offers", "qwen-solo-rtx5090"}, &output, filepath.Join(dir, "config.toml"))
	if err == nil || !strings.Contains(err.Error(), "VAST_API_KEY") {
		t.Fatalf("expected token error, got %v", err)
	}
}

func offerServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v0/bundles" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	previous := vastAPIBaseURL
	vastAPIBaseURL = server.URL
	t.Cleanup(func() { vastAPIBaseURL = previous })
	return server
}

func TestOffersListsRankedTable(t *testing.T) {
	offerServer(t, `{"offers":[{"id":7,"machine_id":9,"gpu_name":"RTX 5090","num_gpus":1,"gpu_ram":32607,"cpu_cores_effective":12,"cpu_ram":64000,"disk_space":200,"inet_down":1000,"inet_up":500,"driver_version":"570.124.06","dph_total":0.42,"geolocation":"FR","reliability":0.95}]}`)
	t.Setenv("VAST_API_KEY", "test-token")
	dir := t.TempDir()
	var output bytes.Buffer
	if err := runWith([]string{"offers", "qwen-solo-rtx5090"}, &output, filepath.Join(dir, "config.toml")); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, want := range []string{"7", "0.42", "307", "RTX 5090", "FR"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q:\n%s", want, text)
		}
	}
}

func TestOffersCapFiltersAboveCap(t *testing.T) {
	offerServer(t, `{"offers":[{"id":7,"machine_id":9,"gpu_name":"RTX 5090","num_gpus":1,"gpu_ram":32607,"cpu_cores_effective":12,"cpu_ram":64000,"disk_space":200,"inet_down":1000,"inet_up":500,"driver_version":"570.124.06","dph_total":1.25,"geolocation":"FR","reliability":0.95}]}`)
	t.Setenv("VAST_API_KEY", "test-token")
	dir := t.TempDir()
	var output bytes.Buffer
	if err := runWith([]string{"offers", "qwen-solo-rtx5090", "--cap", "0.10"}, &output, filepath.Join(dir, "config.toml")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "No eligible offers") {
		t.Fatalf("expected empty result:\n%s", output.String())
	}
}

func TestOffersJSONParses(t *testing.T) {
	offerServer(t, `{"offers":[{"id":7,"machine_id":9,"gpu_name":"RTX 5090","num_gpus":1,"gpu_ram":32607,"cpu_cores_effective":12,"cpu_ram":64000,"disk_space":200,"inet_down":1000,"inet_up":500,"driver_version":"570.124.06","dph_total":0.42,"geolocation":"FR","reliability":0.95}]}`)
	t.Setenv("VAST_API_KEY", "test-token")
	dir := t.TempDir()
	var output bytes.Buffer
	if err := runWith([]string{"offers", "qwen-solo-rtx5090", "--json"}, &output, filepath.Join(dir, "config.toml")); err != nil {
		t.Fatal(err)
	}
	var recommendations []map[string]any
	if err := json.Unmarshal(output.Bytes(), &recommendations); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, output.String())
	}
	if len(recommendations) != 1 {
		t.Fatalf("recommendations = %d, want 1", len(recommendations))
	}
}

func writeStatusFixture(t *testing.T, dir string) state.Deployment {
	t.Helper()
	store, err := state.Load(state.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	deployment := state.Deployment{
		ID: "qwen-solo-rtx5090-20260907t1432", RecipeID: "qwen-solo-rtx5090", RecipeVersion: 1,
		Pins:     state.Pins{ImageDigest: "sha256:abc", ModelRepository: "o/m", ModelRevision: "319f741cce68d7914884900c138a1fbb70a42f30"},
		Instance: state.Instance{ID: 123, Status: "running", Offer: state.OfferSnapshot{GPUName: "RTX 5090", HourlyUSD: 0.42, Location: "FR"}},
		SSH:      state.SSH{Host: "h", Port: 22, User: "root", IdentityFile: "i", KnownHostsFile: "k"},
		Route:    state.Route{LocalHost: "127.0.0.1", LocalPort: 30000, RemoteHost: "127.0.0.1", RemotePort: 30000},
		Spend:    state.Spend{HourlyUSD: 0.42, TotalUSD: 1.26},
		State:    state.Serving,
	}
	if err := store.Add(deployment); err != nil {
		t.Fatal(err)
	}
	if err := store.SetActive(deployment.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(state.Dir(dir)); err != nil {
		t.Fatal(err)
	}
	return deployment
}

func TestStatusShowsActiveDeployment(t *testing.T) {
	dir := t.TempDir()
	deployment := writeStatusFixture(t, dir)
	var output bytes.Buffer
	if err := runWith([]string{"status"}, &output, filepath.Join(state.Dir(dir), "config.toml")); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{deployment.ID, "serving", "127.0.0.1:30000"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("missing %q:\n%s", want, output.String())
		}
	}
}

func TestStatusJSONParses(t *testing.T) {
	dir := t.TempDir()
	deployment := writeStatusFixture(t, dir)
	var output bytes.Buffer
	if err := runWith([]string{"status", deployment.ID, "--json"}, &output, filepath.Join(state.Dir(dir), "config.toml")); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, output.String())
	}
	if decoded["ID"] != deployment.ID {
		t.Fatalf("id = %v", decoded["ID"])
	}
}

func TestStatusWithoutDeploymentsErrors(t *testing.T) {
	dir := t.TempDir()
	var output bytes.Buffer
	err := runWith([]string{"status"}, &output, filepath.Join(state.Dir(dir), "config.toml"))
	if err == nil || !strings.Contains(err.Error(), "no active deployment") {
		t.Fatalf("expected no-active error, got %v", err)
	}
	err = runWith([]string{"status", "missing"}, &output, filepath.Join(state.Dir(dir), "config.toml"))
	if err == nil || !strings.Contains(err.Error(), "unknown deployment") {
		t.Fatalf("expected unknown error, got %v", err)
	}
}

func TestDoctorWithoutActiveDeploymentErrors(t *testing.T) {
	dir := t.TempDir()
	var output bytes.Buffer
	err := runWith([]string{"doctor"}, &output, filepath.Join(state.Dir(dir), "config.toml"))
	if err == nil || !strings.Contains(err.Error(), "no active deployment") {
		t.Fatalf("expected no-active error, got %v", err)
	}
}
