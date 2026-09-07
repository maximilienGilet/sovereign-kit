package vast

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStartInstanceRequiresConfirmedResponse(t *testing.T) {
	for _, body := range []string{`{"success":true}`, `{"success":false}`, `{}`, `{"success":true} {}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				payload, _ := io.ReadAll(r.Body)
				if r.Method != "PUT" || r.URL.Path != "/api/v0/instances/987/" || string(payload) != `{"state":"running"}` || r.Header.Get("Authorization") != "Bearer test" {
					t.Errorf("unexpected start request: %s %s %s", r.Method, r.URL.Path, payload)
				}
				io.WriteString(w, body)
			}))
			defer server.Close()
			starter, ok := any(NewClient(server.URL, "test")).(interface {
				StartInstance(context.Context, int) error
			})
			if !ok {
				t.Fatal("Vast client cannot restart a stopped instance")
			}
			err := starter.StartInstance(context.Background(), 987)
			if (err == nil) != (body == `{"success":true}`) {
				t.Fatalf("start result: %v", err)
			}
		})
	}
}

func TestResourcesUnavailableIsQueuedOnlyWithExplicitStartAcknowledgement(t *testing.T) {
	for _, tc := range []struct {
		body       string
		status     int
		stop, want bool
	}{
		{`{"success":false,"error":"resources_unavailable","msg":"Required resources are currently unavailable, state change queued."}`, 200, false, true},
		{`{"success":false,"error":"resources_unavailable","msg":"Required resources are currently unavailable."}`, 200, false, false},
		{`{"success":false,"error":"permission_denied","msg":"state change queued"}`, 200, false, false},
		{`{"success":false,"error":"resources_unavailable","msg":"state change queued"}`, 403, false, false},
		{`{"error":"resources_unavailable","msg":"state change queued"}`, 200, false, false},
		{`{"success":false,"error":"resources_unavailable","msg":"state change queued"}`, 200, true, false},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); io.WriteString(w, tc.body) }))
		client := NewClient(server.URL, "test")
		var err error
		if tc.stop {
			err = client.StopInstance(context.Background(), 987)
		} else {
			err = client.StartInstance(context.Background(), 987)
		}
		server.Close()
		var queued interface{ Queued() bool }
		got := errors.As(err, &queued) && queued.Queued()
		if err == nil || got != tc.want {
			t.Fatalf("queue classification %q: got %v want %v (%v)", tc.body, got, tc.want, err)
		}
	}
}

func TestStartFailurePreservesProviderReasonWithoutSecretsOrTerminalControls(t *testing.T) {
	for _, status := range []int{200, 400, 403, 429} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				io.WriteString(w, `{"success":false,"error":"restart_denied","msg":"Capacity unavailable for secret-token\u001b[31m","detail":"Contact the host"}`)
			}))
			defer server.Close()
			err := NewClient(server.URL, "secret-token").StartInstance(context.Background(), 987)
			if err == nil || !strings.Contains(err.Error(), "restart_denied") || !strings.Contains(err.Error(), "Capacity unavailable") || !strings.Contains(err.Error(), "Contact the host") {
				t.Fatalf("Vast reason lost: %v", err)
			}
			if strings.Contains(err.Error(), "secret-token") || strings.ContainsAny(err.Error(), "\x1b\n\r") {
				t.Fatalf("unsafe diagnostic: %q", err)
			}
		})
	}
}

func TestMissingSuccessIsNotReportedAsExplicitProviderRefusal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{}`) }))
	defer server.Close()
	err := NewClient(server.URL, "test").StartInstance(context.Background(), 987)
	if err == nil || !strings.Contains(err.Error(), "missing success") {
		t.Fatalf("ambiguous acknowledgement hidden: %v", err)
	}
}
