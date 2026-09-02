package vast

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchOffersReturnsOnlyEligibleOffers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v0/bundles" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatal("missing bearer token")
	}
		body, _ := io.ReadAll(r.Body)
		for _, expected := range []string{`"limit":5`, `"type":"on-demand"`, `"verified":{"eq":true}`, `"gpu_ram":{"gte":96}`} {
			if !strings.Contains(string(body), expected) {
				t.Fatalf("request body missing %s: %s", expected, body)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"offers":[{"id":42,"gpu_name":"RTX PRO 6000","gpu_ram":96,"dph_total":1.25,"geolocation":"FR","reliability":0.99}]}`))
	}))
	defer server.Close()

	offers, err := NewClient(server.URL, "test-token").SearchOffers(context.Background(), SearchRequest{Limit: 5, MinimumVRAMGB: 96})
	if err != nil {
		t.Fatal(err)
	}
	if len(offers) != 1 || offers[0].ID != 42 || offers[0].GPUName != "RTX PRO 6000" {
		t.Fatalf("unexpected offers: %#v", offers)
	}
}
