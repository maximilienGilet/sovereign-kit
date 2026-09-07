package state

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func testDeployment(id string) Deployment {
	when := time.Date(2026, 9, 7, 14, 32, 0, 0, time.UTC)
	return Deployment{
		ID:            id,
		RecipeID:      "qwen-solo-rtx5090",
		RecipeVersion: 1,
		Pins: Pins{
			ImageDigest:     "sha256:a9ceb65277f2f4dfd4cc3b5896f4398703e323249dd9bf87eb20b17b9f467239",
			ModelRepository: "unsloth/Qwen3.8-27B-GGUF",
			ModelRevision:   "4ca720788d1e01f1bff70c033e0d0028fd02e502",
			ModelFilename:   "Qwen3.8-27B-UD-Q4_K_XL.gguf",
			ModelSHA256:     "3f227079003add2511437e5b1e94812e363385225bf6a9b47b0054a72bc8b01e",
		},
		Instance: Instance{
			ID:     123456,
			Status: "running",
			Offer: OfferSnapshot{
				ID: 42, GPUName: "RTX 5090", GPUCount: 1,
				GPUVRAMGB: 32.607, HourlyUSD: 0.42, Location: "FR",
			},
			Image: "ghcr.io/maximiliengilet/sovereign-kit-llama@sha256:a9ceb65277f2f4dfd4cc3b5896f4398703e323249dd9bf87eb20b17b9f467239",
		},
		SSH: SSH{
			Host: "ssh.vast.ai", Port: 22022, User: "root",
			IdentityFile:   IdentityPath(".", id),
			KnownHostsFile: KnownHostsPath(".", id),
		},
		Route:     Route{LocalHost: "127.0.0.1", LocalPort: 30000, RemoteHost: "127.0.0.1", RemotePort: 30000},
		Spend:     Spend{HourlyUSD: 0.42, TotalUSD: 1.26},
		CapUSD:    5,
		State:     Serving,
		CreatedAt: when,
		UpdatedAt: when,
	}
}

func TestSaveLoadRoundTripPreservesDeployment(t *testing.T) {
	dir := t.TempDir()
	store, err := Load(Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	want := testDeployment("qwen-solo-rtx5090-20260907t1432")
	if err := store.Add(want); err != nil {
		t.Fatal(err)
	}
	if err := store.SetActive(want.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(Dir(dir)); err != nil {
		t.Fatal(err)
	}
	got, err := Load(Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	if got.Active != want.ID {
		t.Fatalf("active = %q, want %q", got.Active, want.ID)
	}
	loaded, ok := got.Get(want.ID)
	if !ok {
		t.Fatalf("deployment %q missing after reload", want.ID)
	}
	if loaded.CreatedAt.Unix() != want.CreatedAt.Unix() || loaded.UpdatedAt.Unix() != want.UpdatedAt.Unix() {
		t.Fatalf("timestamps changed: %+v vs %+v", loaded, want)
	}
	loaded.CreatedAt, loaded.UpdatedAt, want.CreatedAt, want.UpdatedAt = time.Time{}, time.Time{}, time.Time{}, time.Time{}
	if !reflect.DeepEqual(loaded, want) {
		t.Fatalf("deployment changed:\n%+v\nvs\n%+v", loaded, want)
	}
}

func TestSaveUsesPrivatePermissions(t *testing.T) {
	dir := Dir(t.TempDir())
	store, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(StatePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state file perm = %o, want 600", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("state dir perm = %o, want 700", dirInfo.Mode().Perm())
	}
}

func TestLoadMissingStoreReturnsDefaults(t *testing.T) {
	store, err := Load(Dir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if store.Version != 1 || len(store.Deployments) != 0 || store.Active != "" {
		t.Fatalf("unexpected fresh store: %+v", store)
	}
	if store.Settings.PortMin != 30000 || store.Settings.PortMax != 30099 || store.Settings.SpendCapUSD != 0 {
		t.Fatalf("unexpected default settings: %+v", store.Settings)
	}
}

func TestLoadRejectsGarbageAndBadState(t *testing.T) {
	dir := Dir(t.TempDir())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(StatePath(dir), []byte("not = [valid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("expected error for garbage TOML")
	}
	store, err := Load(Dir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	bad := testDeployment("bad-state")
	bad.State = "flying"
	if err := store.Add(bad); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(Dir(t.TempDir())); err == nil {
		t.Fatal("expected error for unknown state")
	}
}

func TestAddDuplicateAndUpdateMissingFail(t *testing.T) {
	var store Store
	first := testDeployment("dup")
	if err := store.Add(first); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(first); err == nil {
		t.Fatal("expected duplicate id error")
	}
	if err := store.Update(testDeployment("missing")); err == nil {
		t.Fatal("expected missing deployment error")
	}
	if _, ok := store.Get("missing"); ok {
		t.Fatal("expected Get to report missing")
	}
}

func TestActivePointerLifecycle(t *testing.T) {
	var store Store
	if _, ok := store.ActiveDeployment(); ok {
		t.Fatal("expected no active deployment")
	}
	if err := store.SetActive("missing"); err == nil {
		t.Fatal("expected error activating missing deployment")
	}
	deployment := testDeployment("active-one")
	if err := store.Add(deployment); err != nil {
		t.Fatal(err)
	}
	if err := store.SetActive(deployment.ID); err != nil {
		t.Fatal(err)
	}
	active, ok := store.ActiveDeployment()
	if !ok || active.ID != deployment.ID {
		t.Fatalf("active = %+v, %v", active, ok)
	}
	store.ClearActive()
	if _, ok := store.ActiveDeployment(); ok {
		t.Fatal("expected cleared active pointer")
	}
}

func TestAllocatePortSkipsLiveAndReusesDestroyed(t *testing.T) {
	var store Store
	store.Settings = DefaultSettings()
	first, err := store.AllocatePort()
	if err != nil || first != 30000 {
		t.Fatalf("first port = %d, %v", first, err)
	}
	live := testDeployment("live")
	live.Route.LocalPort = first
	live.State = Tunneled
	if err := store.Add(live); err != nil {
		t.Fatal(err)
	}
	second, err := store.AllocatePort()
	if err != nil || second != 30001 {
		t.Fatalf("second port = %d, %v", second, err)
	}
	dead := testDeployment("dead")
	dead.Route.LocalPort = second
	dead.State = Destroyed
	if err := store.Add(dead); err != nil {
		t.Fatal(err)
	}
	reused, err := store.AllocatePort()
	if err != nil || reused != second {
		t.Fatalf("reused port = %d, %v", reused, err)
	}
	parked := testDeployment("parked")
	parked.Route.LocalPort = second
	parked.State = Stopped
	if err := store.Add(parked); err != nil {
		t.Fatal(err)
	}
	next, err := store.AllocatePort()
	if err != nil || next != second+1 {
		t.Fatalf("next port = %d, %v", next, err)
	}
}

func TestAllocatePortExhaustionErrors(t *testing.T) {
	store := Store{Settings: Settings{PortMin: 30000, PortMax: 30000}}
	taken := testDeployment("taken")
	taken.Route.LocalPort = 30000
	taken.State = Serving
	if err := store.Add(taken); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AllocatePort(); err == nil {
		t.Fatal("expected exhaustion error")
	}
}

func TestNextIDSlugsAndCollides(t *testing.T) {
	var store Store
	now := time.Date(2026, 9, 7, 14, 32, 0, 0, time.UTC)
	if got := store.NextID("qwen-solo-rtx5090", now); got != "qwen-solo-rtx5090-20260907t1432" {
		t.Fatalf("id = %q", got)
	}
	if err := store.Add(testDeployment("qwen-solo-rtx5090-20260907t1432")); err != nil {
		t.Fatal(err)
	}
	if got := store.NextID("qwen-solo-rtx5090", now); got != "qwen-solo-rtx5090-20260907t1432-2" {
		t.Fatalf("collided id = %q", got)
	}
}

func TestTokenPrefersEnvThenFile(t *testing.T) {
	dir := Dir(t.TempDir())
	t.Setenv("VAST_API_KEY", "  env-token  ")
	if got, err := Token(dir); err != nil || got != "env-token" {
		t.Fatalf("token = %q, %v", got, err)
	}
	t.Setenv("VAST_API_KEY", "")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(CredentialsPath(dir), []byte("token = \"file-token\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := Token(dir); err != nil || got != "file-token" {
		t.Fatalf("token = %q, %v", got, err)
	}
	if _, err := Token(Dir(t.TempDir())); err == nil {
		t.Fatal("expected missing credentials error")
	}
}

func TestValidateRejectsBadDeployments(t *testing.T) {
	cases := map[string]func(*Deployment){
		"empty id":          func(d *Deployment) { d.ID = "" },
		"unknown state":     func(d *Deployment) { d.State = "flying" },
		"non-loopback":      func(d *Deployment) { d.Route.LocalHost = "0.0.0.0" },
		"port out of range": func(d *Deployment) { d.Route.LocalPort = 40000 },
		"remote not fixed":  func(d *Deployment) { d.Route.RemotePort = 30001 },
		"bad ssh port":      func(d *Deployment) { d.SSH.Port = 0 },
	}
	for name, mutate := range cases {
		deployment := testDeployment("check")
		mutate(&deployment)
		var store Store
		store.Settings = DefaultSettings()
		if err := store.Add(deployment); err != nil {
			t.Fatal(err)
		}
		if err := store.Validate(); err == nil {
			t.Fatalf("%s: expected validation error", name)
		}
	}
}

func TestLiveStatesOwnInstances(t *testing.T) {
	for state, live := range map[DeploymentState]bool{
		Planned: true, Renting: true, Preparing: true, Serving: true,
		Tunneled: true, Failed: true, Stopped: false, Destroyed: false,
	} {
		if state.Live() != live {
			t.Fatalf("state %q live = %v, want %v", state, state.Live(), live)
		}
	}
}

func TestLayoutHelpers(t *testing.T) {
	dir := Dir(filepath.Join(t.TempDir(), "config"))
	if filepath.Base(dir) != "sovereign-kit" {
		t.Fatalf("dir = %q", dir)
	}
	if filepath.Base(StatePath(dir)) != "state.toml" {
		t.Fatalf("state path = %q", StatePath(dir))
	}
	if filepath.Base(CredentialsPath(dir)) != "credentials.toml" {
		t.Fatalf("credentials path = %q", CredentialsPath(dir))
	}
	if filepath.Base(DeploymentsPath(dir)) != "deployments" {
		t.Fatalf("deployments path = %q", DeploymentsPath(dir))
	}
	if filepath.Base(LegacyConfigPath(dir)) != "config.toml" {
		t.Fatalf("legacy path = %q", LegacyConfigPath(dir))
	}
}

func TestDeploymentPathHelpers(t *testing.T) {
	dir := Dir(t.TempDir())
	if got := DeploymentDir(dir, "x"); got != filepath.Join(dir, "deployments", "x") {
		t.Fatalf("deployment dir = %q", got)
	}
	if got := IdentityPath(dir, "x"); got != filepath.Join(dir, "deployments", "x", "identity") {
		t.Fatalf("identity path = %q", got)
	}
	if got := KnownHostsPath(dir, "x"); got != filepath.Join(dir, "deployments", "x", "known_hosts") {
		t.Fatalf("known hosts path = %q", got)
	}
}
