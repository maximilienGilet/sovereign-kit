package endpointstats

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadMeasuredActivityAndMissingFields(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			t.Errorf("wrong path %s", r.URL.Path)
		}
		fmt.Fprintln(w, "# TYPE llamacpp:requests_processing gauge\nllamacpp:requests_processing 2\nllamacpp:requests_deferred 0\nllamacpp:predicted_tokens_seconds 42.5\nllamacpp:kv_cache_usage_ratio 0.3")
	}))
	defer s.Close()
	got := Read(context.Background(), s.URL+"/v1")
	if got.Problem != "" || got.Active == nil || *got.Active != 2 || got.Queued == nil || *got.Queued != 0 || got.DecodeTokensPerSecond == nil || *got.DecodeTokensPerSecond != 42.5 || got.KVUsageRatio == nil || *got.KVUsageRatio != 0.3 {
		t.Fatalf("snapshot %+v", got)
	}
	if got.PromptTokensPerSecond != nil {
		t.Fatal("missing metric represented as zero")
	}
}

func TestReadRejectsUnsafeDestinationsAndInvalidMetrics(t *testing.T) {
	for _, base := range []string{"http://example.com/v1", "http://127.0.0.1@evil.test/v1", "file:///tmp/model", "http://127.0.0.1/v1?secret=foo"} {
		if got := Read(context.Background(), base); got.Problem == "" {
			t.Fatalf("accepted %s", base)
		}
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "llamacpp:requests_processing NaN\nllamacpp:requests_deferred -1\nllamacpp:kv_cache_usage_ratio 2")
	}))
	defer s.Close()
	got := Read(context.Background(), s.URL+"/v1")
	if got.Active != nil || got.Queued != nil || got.KVUsageRatio != nil {
		t.Fatal("invalid metric became measured")
	}
}

func TestReadDoesNotFollowRedirects(t *testing.T) {
	called := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer target.Close()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer s.Close()
	if got := Read(context.Background(), s.URL+"/v1"); got.Problem == "" || called {
		t.Fatal("redirect followed")
	}
}

func TestReadBoundedPayload(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, strings.Repeat("x", 262145)) }))
	defer s.Close()
	if got := Read(context.Background(), s.URL+"/v1"); got.Problem == "" {
		t.Fatal("oversize metrics accepted")
	}
}
