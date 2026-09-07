package vast

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
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
			`"gpu_ram":{"gte":32000}`,
			`"gpu_name":{"eq":"RTX 5090"}`,
			`"num_gpus":{"eq":1}`,
			`"disk_space":{"gte":100}`,
			`"order":[["dph_total","asc"]]`,
		} {
			if !strings.Contains(string(body), expected) {
				t.Fatalf("request body missing %s: %s", expected, body)
			}
		}
		_, _ = w.Write([]byte(`{"offers":[{"id":42,"machine_id":88,"gpu_name":"RTX 5090","num_gpus":1,"gpu_ram":32607,"cpu_cores_effective":12.5,"cpu_ram":32160,"disk_space":500,"inet_down":750.5,"inet_up":375.25,"driver_version":"570.124.06","dph_total":1.25,"geolocation":"FR","reliability":0.99}]}`))
	}))
	defer server.Close()

	offers, err := NewClient(server.URL, "test-token").SearchOffers(context.Background(), SearchRequest{
		Limit:         5,
		GPUModel:      "RTX 5090",
		GPUCount:      1,
		StrictGPU:     true,
		MinimumVRAMGB: 32,
		MinimumDiskGB: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offers) != 1 {
		t.Fatalf("offers = %#v", offers)
	}
	offer := offers[0]
	if offer.ID != 42 || offer.MachineID != 88 || offer.GPUName != "RTX 5090" || offer.GPUCount != 1 || offer.GPUVRAMGB != 32.607 || offer.TotalGPUVRAMGB != 32.607 {
		t.Fatalf("unexpected GPU details: %#v", offer)
	}
	if offer.CPUCores != 12.5 || offer.CPURAMGB != 32.16 || offer.DiskSpaceGB != 500 || offer.InetDownMBps != 750.5 || offer.InetUpMBps != 375.25 || offer.DriverVersion != "570.124.06" {
		t.Fatalf("unexpected machine details: %#v", offer)
	}
}

func TestSearchOffersSerializesCountryFilterAndReliabilityOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		for key, want := range map[string]string{"geolocation": `{"in":["FR","DE"]}`, "order": `[["reliability","desc"],["dph_total","asc"]]`, "limit": "100", "gpu_name": `{"eq":"RTX 5090"}`, "num_gpus": `{"eq":1}`, "gpu_ram": `{"gte":32000}`, "disk_space": `{"gte":100}`} {
			if string(got[key]) != want {
				t.Errorf("%s=%s want %s", key, got[key], want)
			}
		}
		fmt.Fprint(w, `{"offers":[]}`)
	}))
	defer server.Close()
	_, err := NewClient(server.URL, "token").SearchOffers(context.Background(), SearchRequest{Limit: 100, MinimumVRAMGB: 32, MinimumDiskGB: 100, StrictGPU: true, GPUModel: "RTX 5090", GPUCount: 1, Countries: []string{" fr ", "DE", "fr"}, Sort: SortReliability})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSearchOffersAllCountriesAndInvalidQueries(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "geolocation") {
			t.Errorf("all countries restricted: %s", body)
		}
		fmt.Fprint(w, `{"offers":[]}`)
	}))
	defer server.Close()
	client := NewClient(server.URL, "token")
	request := SearchRequest{Limit: 5, MinimumVRAMGB: 1, MinimumDiskGB: 1}
	if _, err := client.SearchOffers(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.Countries = []string{"not-a-country"}
	if _, err := client.SearchOffers(context.Background(), request); err == nil {
		t.Fatal("invalid country accepted")
	}
	request.Countries = nil
	request.Sort = "bogus"
	if _, err := client.SearchOffers(context.Background(), request); err == nil {
		t.Fatal("invalid sort accepted")
	}
	if calls != 1 {
		t.Fatalf("invalid requests reached server: %d", calls)
	}
}

func TestSearchOffersPreservesMissingVersusZeroMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"offers":[{"id":1},{"id":2,"dph_total":0,"reliability":0},{"id":3,"dph_total":-1,"reliability":-0.1}]}`)
	}))
	defer server.Close()
	offers, err := NewClient(server.URL, "token").SearchOffers(context.Background(), SearchRequest{Limit: 5, MinimumVRAMGB: 1, MinimumDiskGB: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !offers[0].PriceUnknown || !offers[0].ReliabilityUnknown {
		t.Fatal("missing metrics became measured zero")
	}
	if offers[1].PriceUnknown || offers[1].ReliabilityUnknown || offers[1].HourlyUSD != 0 || offers[1].Reliability != 0 {
		t.Fatal("explicit zero became unknown")
	}
	if !offers[2].PriceUnknown || !offers[2].ReliabilityUnknown {
		t.Fatal("negative metrics remained measurable")
	}
	for _, invalid := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		got := (offerResponse{HourlyUSD: &invalid}).normalized()
		if !got.PriceUnknown {
			t.Fatal("nonfinite price remained available")
		}
	}
}

func TestCountryCatalogNormalizesIndependentChoices(t *testing.T) {
	got, err := NormalizeCountries([]string{" fr ", "DE", "fr"})
	if err != nil || !reflect.DeepEqual(got, []string{"FR", "DE"}) {
		t.Fatalf("countries=%v err=%v", got, err)
	}
	for _, code := range []string{"", "ZZ", "EU", "FRANCE", "840", "AC", "TA", "IC", "XK"} {
		if _, err := NormalizeCountries([]string{code}); err == nil {
			t.Fatalf("invalid code accepted: %q", code)
		}
	}
	names := map[string]string{}
	for _, country := range Countries() {
		names[country.Code] = country.Name
	}
	if names["FR"] != "France" || names["DE"] != "Germany" || len(names) < 200 {
		t.Fatalf("incomplete independent catalog: %v", names)
	}
	for location, want := range map[string]string{" fr ": "FR", "Maryland, US": "US", "DE": "DE", "unknown": "", "EU": ""} {
		if got := CountryCode(location); got != want {
			t.Fatalf("CountryCode(%q)=%q want %q", location, got, want)
		}
	}
}

func TestSearchOffersNormalizesNonIntegralVRAM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"offers":[{"id":7,"gpu_name":"A10","gpu_ram":24000,"dph_total":0.4,"geolocation":"US","reliability":0.9}]}`))
	}))
	defer server.Close()

	offers, err := NewClient(server.URL, "test-token").SearchOffers(context.Background(), SearchRequest{Limit: 1, MinimumVRAMGB: 1, MinimumDiskGB: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(offers) != 1 || offers[0].GPUVRAMGB != 24 {
		t.Fatalf("offers = %#v", offers)
	}
}
