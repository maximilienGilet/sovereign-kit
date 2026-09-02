package vast

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchOffersUsesVastWireUnitsAndNormalizesVRAM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v0/bundles" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		for _, expected := range []string{
			`"limit":5`,
			`"type":"ondemand"`,
			`"verified":{"eq":true}`,
			`"rentable":{"eq":true}`,
			`"rented":{"eq":false}`,
			`"gpu_ram":{"gte":98304}`,
		} {
			if !strings.Contains(string(body), expected) {
				t.Fatalf("request body missing %s: %s", expected, body)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"offers":[{"id":42,"gpu_name":"RTX PRO 6000","gpu_ram":98304,"dph_total":1.25,"geolocation":"FR","reliability":0.99}]}`))
	}))
	defer server.Close()

	offers, err := NewClient(server.URL, "test-token").SearchOffers(context.Background(), SearchRequest{Limit: 5, MinimumVRAMGB: 96})
	if err != nil {
		t.Fatal(err)
	}
	if len(offers) != 1 || offers[0].GPUVRAMGB != 96 {
		t.Fatalf("offers = %#v", offers)
	}
}

func TestSearchOffersNormalizesNonIntegralVRAM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"offers":[{"id":7,"gpu_name":"A10","gpu_ram":24000,"dph_total":0.4,"geolocation":"US","reliability":0.9}]}`))
	}))
	defer server.Close()

	offers, err := NewClient(server.URL, "test-token").SearchOffers(context.Background(), SearchRequest{Limit: 1, MinimumVRAMGB: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(offers) != 1 || offers[0].GPUVRAMGB != 23.4375 {
		t.Fatalf("offers = %#v", offers)
	}
}
