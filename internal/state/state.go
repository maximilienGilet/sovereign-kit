// Package state owns the local deployment store: deployment records, the
// active pointer, global settings, and Vast credential resolution. One
// deployment owns a live Vast instance at a time; the store keeps every
// record, including destroyed ones, as spend history.
package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

const (
	StoreVersion = 1

	defaultPortMin = 30000
	defaultPortMax = 30099
)

// DeploymentState is the lifecycle state of one deployment. Every state
// except Stopped and Destroyed owns a live, possibly billed Vast instance.
type DeploymentState string

const (
	Planned   DeploymentState = "planned"
	Renting   DeploymentState = "renting"
	Preparing DeploymentState = "preparing"
	Serving   DeploymentState = "serving"
	Tunneled  DeploymentState = "tunneled"
	Stopped   DeploymentState = "stopped"
	Destroyed DeploymentState = "destroyed"
	Failed    DeploymentState = "failed"
)

// Live reports whether the state owns a live Vast instance. Failed counts
// as live: a failed deployment may still be billed until destroyed.
func (state DeploymentState) Live() bool {
	return state != Stopped && state != Destroyed
}

// OfferSnapshot freezes the rented offer on the record so a deployment stays
// reproducible after the marketplace moves on.
type OfferSnapshot struct {
	ID        int     `toml:"id"`
	GPUName   string  `toml:"gpu_name"`
	GPUCount  int     `toml:"gpu_count"`
	GPUVRAMGB float64 `toml:"gpu_vram_gb"`
	HourlyUSD float64 `toml:"hourly_usd"`
	Location  string  `toml:"location"`
}

// Instance is the rented Vast machine behind a deployment.
type Instance struct {
	ID     int           `toml:"id"`
	Status string        `toml:"status"`
	Offer  OfferSnapshot `toml:"offer"`
	Image  string        `toml:"image"`
}

// SSH holds the remote connection material. Key files live under the
// deployment directory, never inside the record.
type SSH struct {
	Host           string `toml:"host"`
	Port           int    `toml:"port"`
	User           string `toml:"user"`
	IdentityFile   string `toml:"identity_file"`
	KnownHostsFile string `toml:"known_hosts_file"`
}

// Route is the local OpenAI-compatible endpoint of one deployment. Both
// sides stay on loopback; the local port is allocated per deployment while
// the remote server port stays fixed.
type Route struct {
	LocalHost  string `toml:"local_host"`
	LocalPort  int    `toml:"local_port"`
	RemoteHost string `toml:"remote_host"`
	RemotePort int    `toml:"remote_port"`
}

// Pins freezes the exact recipe content a deployment was created from, so it
// stays relaunchable even when the embedded recipes change.
type Pins struct {
	ImageDigest     string `toml:"image_digest"`
	ModelRepository string `toml:"repository"`
	ModelRevision   string `toml:"revision"`
	ModelFilename   string `toml:"filename,omitempty"`
	ModelSHA256     string `toml:"sha256,omitempty"`
}

// Spend tracks money. HourlyUSD is the rented rate; TotalUSD is the
// cumulative billed amount read back from the provider.
type Spend struct {
	HourlyUSD float64 `toml:"hourly_usd"`
	TotalUSD  float64 `toml:"total_usd"`
}

// Deployment is the unit the CLI lists and manages.
type Deployment struct {
	ID            string          `toml:"id"`
	RecipeID      string          `toml:"recipe_id"`
	RecipeVersion int             `toml:"recipe_version"`
	Pins          Pins            `toml:"pins"`
	Instance      Instance        `toml:"instance"`
	SSH           SSH             `toml:"ssh"`
	Route         Route           `toml:"route"`
	Spend         Spend           `toml:"spend"`
	CapUSD        float64         `toml:"cap_usd,omitempty"`
	State         DeploymentState `toml:"state"`
	CreatedAt     time.Time       `toml:"created_at"`
	UpdatedAt     time.Time       `toml:"updated_at"`
}

// Settings are global guardrails. SpendCapUSD of zero means no default cap;
// PortMin/PortMax bound local port allocation.
type Settings struct {
	SpendCapUSD float64 `toml:"spend_cap_usd,omitempty"`
	PortMin     int     `toml:"port_min"`
	PortMax     int     `toml:"port_max"`
}

// Store is the whole state.toml content.
type Store struct {
	Version     int          `toml:"version"`
	Active      string       `toml:"active,omitempty"`
	Deployments []Deployment `toml:"deployments,omitempty"`
	Settings    Settings     `toml:"settings"`
}

// DefaultSettings returns the v1 guardrails: local ports 30000-30099, no
// default spend cap.
func DefaultSettings() Settings {
	return Settings{PortMin: defaultPortMin, PortMax: defaultPortMax}
}

// Dir returns the per-user state directory for a config home.
func Dir(configHome string) string {
	return filepath.Join(configHome, "sovereign-kit")
}

// StatePath returns the store file inside dir.
func StatePath(dir string) string {
	return filepath.Join(dir, "state.toml")
}

// CredentialsPath returns the Vast token file inside dir.
func CredentialsPath(dir string) string {
	return filepath.Join(dir, "credentials.toml")
}

// DeploymentsPath returns the per-deployment secret directory inside dir.
func DeploymentsPath(dir string) string {
	return filepath.Join(dir, "deployments")
}

// DeploymentDir returns the secret directory of one deployment.
func DeploymentDir(dir, id string) string {
	return filepath.Join(DeploymentsPath(dir), id)
}

// IdentityPath returns the private key file of one deployment.
func IdentityPath(dir, id string) string {
	return filepath.Join(DeploymentDir(dir, id), "identity")
}

// KnownHostsPath returns the pinned host-key file of one deployment.
func KnownHostsPath(dir, id string) string {
	return filepath.Join(DeploymentDir(dir, id), "known_hosts")
}

// LegacyConfigPath returns the superseded single-route config file. It is
// never read, only detected to warn.
func LegacyConfigPath(dir string) string {
	return filepath.Join(dir, "config.toml")
}

// Load reads the store, or returns defaults when no store exists yet. A
// present but invalid store is an error, never silently ignored.
func Load(dir string) (Store, error) {
	contents, err := os.ReadFile(StatePath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return Store{Version: StoreVersion, Settings: DefaultSettings()}, nil
		}
		return Store{}, err
	}
	var store Store
	if err := toml.Unmarshal(contents, &store); err != nil {
		return Store{}, fmt.Errorf("parse state: %w", err)
	}
	if store.Settings == (Settings{}) {
		store.Settings = DefaultSettings()
	}
	if err := store.Validate(); err != nil {
		return Store{}, err
	}
	return store, nil
}

// Save validates then writes the store atomically with private permissions.
func (store Store) Save(dir string) error {
	if err := store.Validate(); err != nil {
		return err
	}
	contents, err := toml.Marshal(store)
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, ".state.*.toml")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, StatePath(dir)); err != nil {
		return err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

// Validate rejects stores that would mislead the CLI: unknown versions or
// states, non-loopback routes, out-of-range ports, dangling active pointers.
func (store Store) Validate() error {
	if store.Version != StoreVersion {
		return fmt.Errorf("state version %d is not supported", store.Version)
	}
	if store.Settings.PortMin < 1 || store.Settings.PortMax > 65535 || store.Settings.PortMin > store.Settings.PortMax {
		return fmt.Errorf("invalid port range %d-%d", store.Settings.PortMin, store.Settings.PortMax)
	}
	if store.Settings.SpendCapUSD < 0 {
		return fmt.Errorf("spend cap cannot be negative")
	}
	seen := make(map[string]bool, len(store.Deployments))
	for _, deployment := range store.Deployments {
		if strings.TrimSpace(deployment.ID) == "" {
			return fmt.Errorf("deployment id is required")
		}
		if seen[deployment.ID] {
			return fmt.Errorf("duplicate deployment %q", deployment.ID)
		}
		seen[deployment.ID] = true
		if err := deployment.validate(store.Settings); err != nil {
			return fmt.Errorf("deployment %q: %w", deployment.ID, err)
		}
	}
	if store.Active != "" && !seen[store.Active] {
		return fmt.Errorf("active deployment %q does not exist", store.Active)
	}
	return nil
}

func (deployment Deployment) validate(settings Settings) error {
	switch deployment.State {
	case Planned, Renting, Preparing, Serving, Tunneled, Stopped, Destroyed, Failed:
	default:
		return fmt.Errorf("unknown state %q", deployment.State)
	}
	if strings.TrimSpace(deployment.RecipeID) == "" {
		return fmt.Errorf("recipe id is required")
	}
	if deployment.Route.LocalHost != "127.0.0.1" || deployment.Route.RemoteHost != "127.0.0.1" {
		return fmt.Errorf("route must use loopback (127.0.0.1) on both sides")
	}
	if deployment.Route.LocalPort < settings.PortMin || deployment.Route.LocalPort > settings.PortMax {
		return fmt.Errorf("local port %d outside range %d-%d", deployment.Route.LocalPort, settings.PortMin, settings.PortMax)
	}
	if deployment.Route.RemotePort != 30000 {
		return fmt.Errorf("remote port must be 30000")
	}
	if strings.TrimSpace(deployment.SSH.Host) == "" {
		return fmt.Errorf("SSH host is required")
	}
	if deployment.SSH.Port < 1 || deployment.SSH.Port > 65535 {
		return fmt.Errorf("SSH port must be between 1 and 65535")
	}
	if strings.TrimSpace(deployment.SSH.User) == "" {
		return fmt.Errorf("SSH user is required")
	}
	if strings.TrimSpace(deployment.SSH.IdentityFile) == "" || strings.TrimSpace(deployment.SSH.KnownHostsFile) == "" {
		return fmt.Errorf("SSH identity_file and known_hosts_file are required")
	}
	if deployment.Spend.HourlyUSD < 0 || deployment.Spend.TotalUSD < 0 || deployment.CapUSD < 0 {
		return fmt.Errorf("spend and cap cannot be negative")
	}
	return nil
}

// Get returns the deployment by id.
func (store Store) Get(id string) (Deployment, bool) {
	for _, deployment := range store.Deployments {
		if deployment.ID == id {
			return deployment, true
		}
	}
	return Deployment{}, false
}

// ActiveDeployment returns the deployment the Active pointer names.
func (store Store) ActiveDeployment() (Deployment, bool) {
	if store.Active == "" {
		return Deployment{}, false
	}
	return store.Get(store.Active)
}

// Add inserts a deployment; duplicate ids are rejected.
func (store *Store) Add(deployment Deployment) error {
	if _, ok := store.Get(deployment.ID); ok {
		return fmt.Errorf("deployment %q already exists", deployment.ID)
	}
	store.Deployments = append(store.Deployments, deployment)
	return nil
}

// Update replaces a deployment by id and stamps it.
func (store *Store) Update(deployment Deployment) error {
	for index := range store.Deployments {
		if store.Deployments[index].ID == deployment.ID {
			deployment.UpdatedAt = time.Now().UTC()
			store.Deployments[index] = deployment
			return nil
		}
	}
	return fmt.Errorf("deployment %q does not exist", deployment.ID)
}

// SetActive points at an existing deployment.
func (store *Store) SetActive(id string) error {
	if _, ok := store.Get(id); !ok {
		return fmt.Errorf("deployment %q does not exist", id)
	}
	store.Active = id
	return nil
}

// ClearActive drops the pointer without touching records.
func (store *Store) ClearActive() {
	store.Active = ""
}

// AllocatePort returns the first local port no surviving deployment holds.
// Only Destroyed records release theirs: a stopped deployment keeps its
// port so resume restores the same local endpoint its consumers use.
func (store Store) AllocatePort() (int, error) {
	held := make(map[int]bool, len(store.Deployments))
	for _, deployment := range store.Deployments {
		if deployment.State != Destroyed {
			held[deployment.Route.LocalPort] = true
		}
	}
	for port := store.Settings.PortMin; port <= store.Settings.PortMax; port++ {
		if !held[port] {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free local port in range %d-%d", store.Settings.PortMin, store.Settings.PortMax)
}

// NextID slugs a deployment id from the recipe and timestamp, suffixing on
// collision so ids stay human-readable and unique.
func (store Store) NextID(recipeID string, now time.Time) string {
	base := fmt.Sprintf("%s-%s", recipeID, now.UTC().Format("20060102t1504"))
	if _, ok := store.Get(base); !ok {
		return base
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s-%d", base, n)
		if _, ok := store.Get(candidate); !ok {
			return candidate
		}
	}
}

// Token resolves the Vast API token: VAST_API_KEY first, then the private
// credentials file. It never invents one.
func Token(dir string) (string, error) {
	if token := strings.TrimSpace(os.Getenv("VAST_API_KEY")); token != "" {
		return token, nil
	}
	contents, err := os.ReadFile(CredentialsPath(dir))
	if err != nil {
		return "", fmt.Errorf("set VAST_API_KEY or add a token to %s", CredentialsPath(dir))
	}
	var credentials struct {
		Token string `toml:"token"`
	}
	if err := toml.Unmarshal(contents, &credentials); err != nil {
		return "", fmt.Errorf("parse credentials: %w", err)
	}
	if strings.TrimSpace(credentials.Token) == "" {
		return "", fmt.Errorf("set VAST_API_KEY or add a token to %s", CredentialsPath(dir))
	}
	return strings.TrimSpace(credentials.Token), nil
}
