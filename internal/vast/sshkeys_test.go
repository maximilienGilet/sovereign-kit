package vast

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListSSHKeysReadsArrayAndEnvelope(t *testing.T) {
	array := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v0/ssh/" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"id":7,"user_id":1,"key":"ssh-ed25519 QUFBQQ=="}]`))
	}))
	defer array.Close()
	keys, err := NewClient(array.URL, "test-token").ListSSHKeys(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].ID != 7 || keys[0].Key != "ssh-ed25519 QUFBQQ==" {
		t.Fatalf("keys = %+v", keys)
	}

	envelope := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ssh_keys":[{"id":9,"public_key":"ssh-ed25519 AQkJCQg=="}]}`))
	}))
	defer envelope.Close()
	keys, err = NewClient(envelope.URL, "test-token").ListSSHKeys(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].ID != 9 || keys[0].Key != "ssh-ed25519 AQkJCQg==" {
		t.Fatalf("keys = %+v", keys)
	}
}

func TestListSSHKeysTreatsNotFoundAsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	keys, err := NewClient(server.URL, "test-token").ListSSHKeys(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 0 {
		t.Fatalf("keys = %+v", keys)
	}
}

func TestDeleteSSHKeyRemovesByID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v0/ssh/7" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()
	if err := NewClient(server.URL, "test-token").DeleteSSHKey(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteSSHKeyRejectsBadIDAndFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"success":false}`))
	}))
	defer server.Close()
	client := NewClient(server.URL, "test-token")
	if err := client.DeleteSSHKey(context.Background(), 0); err == nil {
		t.Fatal("expected error for non-positive id")
	}
	if err := client.DeleteSSHKey(context.Background(), 7); err == nil {
		t.Fatal("expected error for failed delete")
	}
}

func TestHasSSHKeyStillFindsListedKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":7,"ssh_key":"ssh-ed25519 QUFBQQ=="}]`))
	}))
	defer server.Close()
	found, err := NewClient(server.URL, "test-token").HasSSHKey(context.Background(), "ssh-ed25519 QUFBQQ==")
	if err != nil || !found {
		t.Fatalf("found = %v, %v", found, err)
	}
}
