package vast

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchOffersRequestsBidTypeWhenInterruptible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"type":"bid"`) {
			t.Fatalf("request body missing bid type: %s", body)
		}
		if strings.Contains(string(body), `"type":"ondemand"`) {
			t.Fatalf("request body must not mix types: %s", body)
		}
		_, _ = w.Write([]byte(`{"offers":[]}`))
	}))
	defer server.Close()

	offers, err := NewClient(server.URL, "test-token").SearchOffers(context.Background(), SearchRequest{
		Limit:         5,
		StrictGPU:     true,
		GPUModel:      "RTX 5090",
		GPUCount:      1,
		MinimumVRAMGB: 32,
		MinimumDiskGB: 100,
		Interruptible: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offers) != 0 {
		t.Fatalf("offers = %#v", offers)
	}
}

func TestSearchOffersDefaultsToOnDemand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"type":"ondemand"`) {
			t.Fatalf("request body missing ondemand type: %s", body)
		}
		_, _ = w.Write([]byte(`{"offers":[]}`))
	}))
	defer server.Close()

	if _, err := NewClient(server.URL, "test-token").SearchOffers(context.Background(), SearchRequest{
		Limit:         5,
		MinimumVRAMGB: 32,
		MinimumDiskGB: 100,
	}); err != nil {
		t.Fatal(err)
	}
}
