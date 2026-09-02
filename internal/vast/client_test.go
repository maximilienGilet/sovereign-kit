package vast

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateInstanceUsesSSHDirectWireContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v0/asks/42" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		for _, expected := range []string{
			`"disk":120`,
			`"runtype":"ssh_direct"`,
			`"label":"sovkit-qwen-studio"`,
		} {
			if !strings.Contains(string(body), expected) {
				t.Fatalf("request body missing %s: %s", expected, body)
			}
		}
		if strings.Contains(string(body), "onstart") {
			t.Fatalf("create request must not start code before host trust: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"new_contract":987}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	instanceID, err := client.CreateInstance(context.Background(), 42, CreateRequest{
		Image:  "lmsysorg/sglang@sha256:e21dd539b36ea7842101393ec3fe3b0d453626cd8251e4d4db33af0cd97d7f0b",
		DiskGB: 120,
		Label:  "sovkit-qwen-studio",
	})
	if err != nil {
		t.Fatal(err)
	}
	if instanceID != 987 {
		t.Fatalf("instance id = %d, want 987", instanceID)
	}
}

func TestCreateInstanceRejectsMissingToken(t *testing.T) {
	_, err := NewClient("https://console.vast.ai", "").CreateInstance(context.Background(), 42, CreateRequest{})
	if err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("expected token error, got %v", err)
	}
}
