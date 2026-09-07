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

func TestTurboQuantMTPRecipeRequiresExactVLLMContract(t *testing.T) {
	valid := Recipe{
		Version: 1, ID: "qwen-solo-lab", Name: "Qwen Solo Lab", Kind: "speculative-text-generation",
		Profile: Profile{Status: "experimental", Summary: "Qwen3.8 on one RTX 5090", Evidence: "Requires live gauntlet"},
		Runtime: Runtime{
			Engine: "vllm", Image: "ghcr.io/example/vllm@sha256:e21dd539b36ea7842101393ec3fe3b0d453626cd8251e4d4db33af0cd97d7f0b",
			Quantization: "modelopt", KVCacheDType: "turboquant_4bit_nc", GPUMemoryUtilization: 0.94, DisableAsyncScheduling: true,
		},
		Model:       Model{Repository: "gittensor-model-hub/Qwen3.8-27B-NVFP4-RTX5090-LMHead4", Revision: "f7866ff274f7b2ab34d9d828a7bccff51919b711", TrustRemoteCode: true},
		Speculative: &Speculative{Algorithm: "qwen3_5_mtp", NumDraftTokens: 4},
		Serve:       Serve{ContextWindow: 200000, MaxOutputTokens: 16384, MaxRunningRequests: 1},
		Requirements: Requirements{
			GPUModel: "RTX 5090", GPUCount: 1, StrictGPU: true, MinimumVRAMGB: 32, MinimumDiskGB: 100,
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid vLLM contract rejected: %v", err)
	}
	for _, mutate := range []func(*Recipe){
		func(r *Recipe) { r.Runtime.KVCacheDType = "" },
		func(r *Recipe) { r.Runtime.Quantization = "" },
		func(r *Recipe) { r.Runtime.GPUMemoryUtilization = 0 },
		func(r *Recipe) { r.Runtime.DisableAsyncScheduling = false },
		func(r *Recipe) { r.Speculative.NumDraftTokens = 0 },
	} {
		candidate := valid
		speculative := *valid.Speculative
		candidate.Speculative = &speculative
		mutate(&candidate)
		if err := candidate.Validate(); err == nil {
			t.Fatalf("invalid vLLM contract accepted: %#v", candidate)
		}
	}
}
