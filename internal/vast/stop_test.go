package vast

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStopInstanceWireContractAndConfirmation(t *testing.T) {
	for _, body := range []string{`{"success":true}`, `{"success":false}`, `{}`, `{"success":true} {}`, `invalid`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				payload, _ := io.ReadAll(r.Body)
				if r.Method != http.MethodPut || r.URL.Path != "/api/v0/instances/987/" || string(payload) != `{"state":"stopped"}` || r.Header.Get("Authorization") != "Bearer test-token" || r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("unexpected request: %s %s %s", r.Method, r.URL.Path, payload)
				}
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			err := NewClient(server.URL, "test-token").StopInstance(context.Background(), 987)
			if (err == nil) != (body == `{"success":true}`) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestStopInstanceRejectsInvalidInputAndHTTPFailure(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()
	if err := NewClient(server.URL, "test-token").StopInstance(context.Background(), 0); err == nil {
		t.Fatal("invalid id accepted")
	}
	if err := NewClient(server.URL, " ").StopInstance(context.Background(), 987); err == nil {
		t.Fatal("empty token accepted")
	}
	if requests != 0 {
		t.Fatal("invalid input issued HTTP request")
	}
	if err := NewClient(server.URL, "test-token").StopInstance(context.Background(), 987); err == nil {
		t.Fatal("HTTP failure accepted")
	}
}

func TestGetInstanceRejectsTrailingDataBeforeStopConfirmation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"instances":{"id":987,"actual_status":"exited"}} {"unexpected":true}`))
	}))
	defer server.Close()
	if _, err := NewClient(server.URL, "test-token").GetInstance(context.Background(), 987); err == nil {
		t.Fatal("malformed response could confirm stopped instance")
	}
}
