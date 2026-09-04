package clientprofile

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscoveryUsesUniqueActualModelAndMatchingMetadata(t *testing.T) {
	for _, tc := range []struct {
		body, id string
		valid    bool
	}{
		{`{"data":[{"id":"owner/solo"}]}`, "owner/solo", true},
		{`{"data":[{"id":"a"},{"id":"b"}]}`, "", false},
		{`{"data":[]}`, "", false}, {`not json`, "", false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/models" {
					t.Errorf("path %s", r.URL.Path)
				}
				w.Write([]byte(tc.body))
			}))
			defer server.Close()
			got := Discover(context.Background(), server.URL+"/v1", Metadata{ID: "owner/solo", ContextWindow: 32768, MaxTokens: 4096})
			if got.ID != tc.id || (got.Problem == "") != tc.valid {
				t.Fatalf("got %#v", got)
			}
			if tc.valid && (!got.Installable() || got.ContextWindow != 32768) {
				t.Fatalf("missing verified limits: %#v", got)
			}
		})
	}
}
func TestDiscoveryFailsClosedForRedirectCancellationAndMetadataMismatch(t *testing.T) {
	remoteCalls := 0
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { remoteCalls++ }))
	defer remote.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, remote.URL, http.StatusFound) }))
	defer server.Close()
	if got := Discover(context.Background(), server.URL+"/v1", Metadata{}); got.Problem == "" || remoteCalls != 0 {
		t.Fatalf("redirect followed: %#v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := Discover(ctx, server.URL+"/v1", Metadata{}); got.Problem == "" {
		t.Fatal("cancel ignored")
	}
	models := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"data":[{"id":"custom/model"}]}`)) }))
	defer models.Close()
	got := Discover(context.Background(), models.URL+"/v1", Metadata{ID: "studio", ContextWindow: 262144, MaxTokens: 16384})
	if got.ID != "custom/model" || got.Installable() || got.LimitsProblem == "" {
		t.Fatalf("invented limits %#v", got)
	}
}
