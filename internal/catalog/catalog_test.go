package catalog

import "testing"

func TestClassifyDraftModelRequiresSpeculativeRecipe(t *testing.T) {
	result := Classify(Artifact{Repository: "incoai/Qwen3.8-27B-DFlash2", Tags: []string{"text-generation", "draft-model", "speculative-decoding"}})
	if result.Kind != "speculative-text-generation" || !result.RequiresTarget || result.Status != NeedsRecipe {
		t.Fatalf("unexpected draft classification: %#v", result)
	}
}

func TestClassifyGGUFRecommendsLlamaCpp(t *testing.T) {
	result := Classify(Artifact{Repository: "org/model-GGUF", Files: []string{"model-Q4_K_M.gguf"}})
	if result.Kind != "gguf-text-generation" || result.Engine != "llama-cpp" || result.Status != Supported {
		t.Fatalf("unexpected GGUF classification: %#v", result)
	}
}

func TestClassifyUnknownRemoteCodeIsNotAutomaticallyDeployed(t *testing.T) {
	result := Classify(Artifact{Repository: "org/custom", Tags: []string{"text-generation"}, RequiresRemoteCode: true})
	if result.Status != NeedsConfirmation || result.Kind != "text-generation" {
		t.Fatalf("unexpected remote-code classification: %#v", result)
	}
}
