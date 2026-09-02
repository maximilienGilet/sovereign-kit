package huggingface

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInspectResolvesImmutableRevisionAndClassifiesDraftModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/models/incoai/Qwen3.8-27B-DFlash2" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"incoai/Qwen3.8-27B-DFlash2","sha":"319f741cce68d7914884900c138a1fbb70a42f30","pipeline_tag":"text-generation","tags":["draft-model","speculative-decoding"],"siblings":[{"rfilename":"model.safetensors"}]}`))
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
