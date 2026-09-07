package vast

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDestroyInstanceUsesExactWireContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete || request.URL.Path != "/api/v0/instances/987" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		if request.ContentLength != 0 {
			t.Fatalf("DELETE body length = %d", request.ContentLength)
		}
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	if err := NewClient(server.URL, "test-token").DestroyInstance(context.Background(), 987); err != nil {
		t.Fatal(err)
	}
}

func TestDestroyInstanceRejectsUnconfirmedResponses(t *testing.T) {
	for name, response := range map[string]struct {
		status int
		body   string
	}{
		"rejected":   {http.StatusOK, `{"success":false}`},
		"malformed":  {http.StatusOK, `{`},
		"trailing":   {http.StatusOK, `{"success":true}{}`},
		"bad status": {http.StatusAccepted, `{"success":true}`},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(response.status)
				_, _ = w.Write([]byte(response.body))
			}))
			defer server.Close()
			if err := NewClient(server.URL, "test-token").DestroyInstance(context.Background(), 987); err == nil {
				t.Fatal("unconfirmed DELETE succeeded")
			}
		})
	}
}

func TestDestroyInstanceValidatesBeforeHTTP(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests++ }))
	defer server.Close()

	if err := NewClient(server.URL, "test-token").DestroyInstance(context.Background(), 0); err == nil || requests != 0 {
		t.Fatalf("error=%v requests=%d", err, requests)
	}
}

func TestInstanceExistsUsesTargetedOwnerLookup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/api/v0/instances/987/" || request.URL.RawQuery != "owner=me" {
			t.Fatalf("request = %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
		}
		if request.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"instances":{"id":987}}`))
	}))
	defer server.Close()

	exists, err := NewClient(server.URL, "test-token").InstanceExists(context.Background(), 987)
	if err != nil || !exists {
		t.Fatalf("exists=%v err=%v", exists, err)
	}
}

func TestInstanceExistsOnlyTreatsExplicitNullAsAbsence(t *testing.T) {
	for name, response := range map[string]struct {
		status int
		body   string
		want   bool
		fail   bool
	}{
		"absent":     {http.StatusOK, `{"instances":null}`, false, false},
		"wrong id":   {http.StatusOK, `{"instances":{"id":123}}`, false, true},
		"missing id": {http.StatusOK, `{"instances":{}}`, false, true},
		"malformed":  {http.StatusOK, `{`, false, true},
		"not found":  {http.StatusNotFound, ``, false, true},
		"forbidden":  {http.StatusForbidden, ``, false, true},
		"present":    {http.StatusOK, `{"instances":{"id":987}}`, true, false},
		"trailing":   {http.StatusOK, `{"instances":null}{"instances":{"id":987}}`, false, true},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(response.status)
				_, _ = w.Write([]byte(response.body))
			}))
			defer server.Close()
			exists, err := NewClient(server.URL, "test-token").InstanceExists(context.Background(), 987)
			if response.fail {
				if err == nil || exists {
					t.Fatalf("exists=%v err=%v", exists, err)
				}
				return
			}
			if err != nil || exists != response.want {
				t.Fatalf("exists=%v err=%v", exists, err)
			}
		})
	}
}

func TestLifecycleAdapterReturnsNetworkErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpoint := server.URL
	server.Close()
	client := NewClient(endpoint, "test-token")
	if err := client.DestroyInstance(context.Background(), 987); err == nil {
		t.Fatal("network failure destroyed instance")
	}
	if exists, err := client.InstanceExists(context.Background(), 987); err == nil || exists {
		t.Fatalf("exists=%v err=%v", exists, err)
	}
}

func TestInstanceExistsValidatesPositiveIDBeforeHTTP(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	exists, err := NewClient(server.URL, "test-token").InstanceExists(context.Background(), 0)
	if err == nil || exists || requests != 0 {
		t.Fatalf("exists=%v err=%v requests=%d", exists, err, requests)
	}
}

func TestInstanceExistsRejectsMissingTokenBeforeHTTP(t *testing.T) {
	exists, err := NewClient("http://127.0.0.1:1", " ").InstanceExists(context.Background(), 987)
	if err == nil || exists || !strings.Contains(err.Error(), "token") {
		t.Fatalf("exists=%v err=%v", exists, err)
	}
}
