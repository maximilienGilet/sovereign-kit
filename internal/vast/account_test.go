package vast

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBalanceReadsCurrentUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v0/users/current/" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q", got)
		}
		fmt.Fprint(w, `{"balance":42.125}`)
	}))
	defer server.Close()

	client := &Client{baseURL: server.URL, token: "test-token", http: server.Client()}
	got, err := client.Balance(context.Background())
	if err != nil || got != 42.125 {
		t.Fatalf("Balance() = %v, %v", got, err)
	}
}

func TestBalanceAcceptsLegacyCredit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"credit":7.5}`)
	}))
	defer server.Close()
	client := &Client{baseURL: server.URL, token: "test-token", http: server.Client()}
	got, err := client.Balance(context.Background())
	if err != nil || got != 7.5 {
		t.Fatalf("Balance() = %v, %v", got, err)
	}
}

func TestBalancePrefersSpendableCreditWhenBothFieldsArePresent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"balance":0,"credit":6.75}`)
	}))
	defer server.Close()
	client := &Client{baseURL: server.URL, token: "test-token", http: server.Client()}
	got, err := client.Balance(context.Background())
	if err != nil || got != 6.75 {
		t.Fatalf("Balance() = %v, %v; want displayed Vast credit 6.75", got, err)
	}
}

func TestBalanceRejectsInvalidResponses(t *testing.T) {
	for name, body := range map[string]string{
		"missing":    `{}`,
		"negative":   `{"balance":-1}`,
		"not-number": `{"balance":"NaN"}`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			client := &Client{baseURL: server.URL, token: "test-token", http: server.Client()}
			if _, err := client.Balance(context.Background()); err == nil {
				t.Fatal("invalid balance accepted")
			}
		})
	}
}

func TestBalanceRejectsMissingTokenAndHTTPFailure(t *testing.T) {
	if _, err := NewClient("https://console.vast.ai", "").Balance(context.Background()); err == nil {
		t.Fatal("empty token accepted")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "secret upstream diagnostic", http.StatusForbidden)
	}))
	defer server.Close()
	client := &Client{baseURL: server.URL, token: "test-token", http: server.Client()}
	_, err := client.Balance(context.Background())
	if err == nil || !strings.Contains(err.Error(), "HTTP 403") || strings.Contains(err.Error(), "secret upstream diagnostic") {
		t.Fatalf("unsafe or missing error: %v", err)
	}
}

func TestBalanceRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"balance":1,"padding":"%s"}`, strings.Repeat("x", 300000))
	}))
	defer server.Close()
	client := &Client{baseURL: server.URL, token: "test-token", http: server.Client()}
	if _, err := client.Balance(context.Background()); err == nil {
		t.Fatal("oversized account response accepted")
	}
}
