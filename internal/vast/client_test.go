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
func TestCreateInstanceRejectsInvalidRequestsBeforeHTTP(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	validImage := "lmsysorg/sglang@sha256:e21dd539b36ea7842101393ec3fe3b0d453626cd8251e4d4db33af0cd97d7f0b"
	for _, test := range []struct {
		name    string
		request CreateRequest
		want    string
	}{
		{name: "blank image", request: CreateRequest{DiskGB: 120}, want: "image is required"},
		{name: "mutable image", request: CreateRequest{Image: "lmsysorg/sglang:latest", DiskGB: 120}, want: "image must be pinned by sha256 digest"},
		{name: "non-positive disk", request: CreateRequest{Image: validImage, DiskGB: 0}, want: "disk size must be positive"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := client.CreateInstance(context.Background(), 42, test.request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if requests != 0 {
				t.Fatalf("invalid request reached HTTP: %d requests", requests)
			}
		})
	}
}
