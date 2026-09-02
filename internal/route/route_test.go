package route

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
)

func writeRegularFile(t *testing.T, dir, name, contents string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHealthcheckGetsModelsWithoutAuthorization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/v1/models" {
			t.Errorf("path = %q, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q, want no authorization header", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := Healthcheck(context.Background(), server.URL); err != nil {
		t.Fatal(err)
	}
}

func TestHealthcheckUsesModelsPathWithTrailingBaseURLSlash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := Healthcheck(context.Background(), server.URL+"/"); err != nil {
		t.Fatal(err)
	}
}

func TestHealthcheckRejectsNon2xxResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	if err := Healthcheck(context.Background(), server.URL); err == nil {
		t.Fatal("Healthcheck() error = nil, want non-2xx response error")
	}
}

func TestCommandRejectsUnreadableOrNonRegularCredentialFiles(t *testing.T) {
	dir := t.TempDir()
	identity := writeRegularFile(t, dir, "identity", "private key")
	knownHosts := writeRegularFile(t, dir, "known_hosts", "gpu.example.test ssh-ed25519 AAAA")

	t.Run("unreadable identity", func(t *testing.T) {
		if err := os.Chmod(identity, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(identity, 0o600) })

		cfg := config.Studio("gpu.example.test", 22, "sovkit", identity, knownHosts)
		cmd, err := Command(cfg)
		if err == nil {
			t.Fatalf("Command() command = %#v, want unreadable identity error", cmd)
		}
	})

	t.Run("non-regular known hosts", func(t *testing.T) {
		directory := filepath.Join(dir, "known-hosts-directory")
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}

		cfg := config.Studio("gpu.example.test", 22, "sovkit", knownHosts, directory)
		cmd, err := Command(cfg)
		if err == nil {
			t.Fatalf("Command() command = %#v, want non-regular known hosts error", cmd)
		}
	})
}

func TestCommandBuildsPinnedLoopbackSSHForward(t *testing.T) {
	dir := t.TempDir()
	identity := writeRegularFile(t, dir, "identity", "private key")
	knownHosts := writeRegularFile(t, dir, "known_hosts", "gpu.example.test ssh-ed25519 AAAA")
	cfg := config.Studio("gpu.example.test", 22022, "sovkit", identity, knownHosts)

	cmd, err := Command(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(cmd.Path) != "ssh" {
		t.Fatalf("command path = %q, want ssh executable", cmd.Path)
	}
	want := []string{
		"ssh",
		"-N",
		"-o", "BatchMode=yes",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "IdentitiesOnly=yes",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UserKnownHostsFile=" + knownHosts,
		"-i", identity,
		"-L", "127.0.0.1:30000:127.0.0.1:30000",
		"-p", "22022",
		"sovkit@gpu.example.test",
	}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("command args = %#v, want %#v", cmd.Args, want)
	}
}
