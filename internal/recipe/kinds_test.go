package recipe

import "testing"

func TestSpeculativeRecipeRequiresPinnedDraftModel(t *testing.T) {
	r := Recipe{
		Version: 1, ID: "qwen-dflash", Name: "Qwen with DFlash", Kind: "speculative-text-generation",
		Runtime: Runtime{Engine: "sglang", Image: "example/sglang@sha256:e21dd539b36ea7842101393ec3fe3b0d453626cd8251e4d4db33af0cd97d7f0b"},
		Model:   Model{Repository: "Qwen/Qwen3.8-27B", Revision: "319f741cce68d7914884900c138a1fbb70a42f30"},
		Serve:   Serve{ContextWindow: 32768, MaxOutputTokens: 4096, MaxRunningRequests: 1},
	}
	if err := r.Validate(); err == nil {
		t.Fatal("expected missing speculative draft model to be rejected")
	}

	r.Speculative = &Speculative{Algorithm: "dflash", DraftRepository: "incoai/Qwen3.8-27B-DFlash2", DraftRevision: "319f741cce68d7914884900c138a1fbb70a42f30", NumDraftTokens: 8}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestGGUFRecipeRequiresLlamaCppAndASelectedFile(t *testing.T) {
	r := Recipe{
		Version: 1, ID: "small-gguf", Name: "Small GGUF", Kind: "gguf-text-generation",
		Runtime: Runtime{Engine: "llama-cpp", Image: "example/llama-cpp@sha256:e21dd539b36ea7842101393ec3fe3b0d453626cd8251e4d4db33af0cd97d7f0b"},
		Model:   Model{Repository: "org/model-GGUF", Revision: "319f741cce68d7914884900c138a1fbb70a42f30"},
		Serve:   Serve{ContextWindow: 8192, MaxOutputTokens: 1024, MaxRunningRequests: 1},
	}
	if err := r.Validate(); err == nil {
		t.Fatal("expected absent GGUF filename to be rejected")
	}
	r.Model.Filename = "model-Q4_K_M.gguf"
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}
