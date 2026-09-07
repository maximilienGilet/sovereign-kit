package huggingface

import (
	"context"
	"github.com/maximilienGilet/sovereign-kit/internal/catalog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInspectResolvesImmutableRevisionAndClassifiesDraftModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/models/incoai/Qwen3.8-27B-DFlash2":
			_, _ = w.Write([]byte(`{"id":"incoai/Qwen3.8-27B-DFlash2","sha":"319f741cce68d7914884900c138a1fbb70a42f30","pipeline_tag":"text-generation","tags":["draft-model","speculative-decoding"],"siblings":[{"rfilename":"model.safetensors"}]}`))
		case "/incoai/Qwen3.8-27B-DFlash2/resolve/319f741cce68d7914884900c138a1fbb70a42f30/config.json":
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	model, err := NewClient(server.URL).Inspect(context.Background(), "incoai/Qwen3.8-27B-DFlash2")
	if err != nil {
		t.Fatal(err)
	}
	if model.Revision != "319f741cce68d7914884900c138a1fbb70a42f30" {
		t.Fatalf("revision = %q", model.Revision)
	}
	if !model.Classification.RequiresTarget || model.Classification.Kind != "speculative-text-generation" {
		t.Fatalf("unexpected classification: %#v", model.Classification)
	}
}

func TestInspectDetectsRemoteCodeFromPinnedConfig(t *testing.T) {
	const revision = "319f741cce68d7914884900c138a1fbb70a42f30"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/models/acme/custom-model":
			_, _ = w.Write([]byte(`{"id":"acme/custom-model","sha":"` + revision + `","pipeline_tag":"text-generation","siblings":[{"rfilename":"config.json"}]}`))
		case "/acme/custom-model/resolve/" + revision + "/config.json":
			_, _ = w.Write([]byte(`{"auto_map":{"AutoModelForCausalLM":"modeling_custom.CustomModel"}}`))
		default:
			t.Fatalf("unexpected request: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	model, err := NewClient(server.URL).Inspect(context.Background(), "acme/custom-model")
	if err != nil {
		t.Fatal(err)
	}
	if model.Classification.Status != catalog.NeedsConfirmation {
		t.Fatalf("classification=%#v", model.Classification)
	}
}

func TestSearchReturnsRankedModelChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/api/models" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("search") != "qwen coder" || query.Get("sort") != "downloads" || query.Get("direction") != "-1" || query.Get("limit") != "10" {
			t.Fatalf("unexpected query: %s", request.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`[
			{"id":"Qwen/Qwen3-Coder-30B-A3B-Instruct","downloads":1200,"likes":75},
			{"id":"Qwen/Qwen2.5-Coder-32B-Instruct","downloads":900,"likes":60}
		]`))
	}))
	defer server.Close()

	results, err := NewClient(server.URL).Search(context.Background(), "qwen coder", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Repository != "Qwen/Qwen3-Coder-30B-A3B-Instruct" || results[0].Downloads != 1200 || results[0].Likes != 75 {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestSearchRejectsEmptyQueryBeforeHTTP(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests++
	}))
	defer server.Close()

	_, err := NewClient(server.URL).Search(context.Background(), " ", 10)
	if err == nil || requests != 0 {
		t.Fatalf("error=%v requests=%d", err, requests)
	}
}
