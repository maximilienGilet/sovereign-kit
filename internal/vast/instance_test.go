package vast

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetInstanceReturnsSSHConnectionAfterCreation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v0/instances/987" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatal("missing bearer token")
		}
		_, _ = w.Write([]byte(`{"instances":{"id":987,"actual_status":"running","ssh_host":"ssh2281.vast.ai","ssh_port":10882}}`))
	}))
	defer server.Close()

	instance, err := NewClient(server.URL, "test-token").GetInstance(context.Background(), 987)
	if err != nil {
		t.Fatal(err)
	}
	if instance.ID != 987 || instance.Status != "running" || instance.SSHHost != "ssh2281.vast.ai" || instance.SSHPort != 10882 {
		t.Fatalf("unexpected instance: %#v", instance)
	}
}
