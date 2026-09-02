package recipe

import (
	"path/filepath"
	"testing"
)

func TestLoadBundledQwenStudioRecipe(t *testing.T) {
	r, err := Load(filepath.Join("..", "..", "recipes", "qwen-studio.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "qwen-studio" || r.Runtime.Engine != "sglang" || r.Model.Repository != "RadixArk/Qwen3.8-27B-NVFP4" {
		t.Fatalf("unexpected recipe: %#v", r)
	}
	if r.Requirements.MinimumVRAMGB != 96 || r.Requirements.MinimumDiskGB != 120 || r.Serve.ContextWindow != 262144 {
		t.Fatalf("unexpected recipe limits: %#v", r)
	}
}

func TestCustomHuggingFaceRecipeRequiresPinnedRevision(t *testing.T) {
	_, err := CustomHuggingFace("org/model", "main", false)
	if err == nil {
		t.Fatal("expected unpinned revision to be rejected")
	}

	r, err := CustomHuggingFace("org/model", "319f741cce68d7914884900c138a1fbb70a42f30", true)
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "custom-huggingface" || !r.Model.TrustRemoteCode {
		t.Fatalf("unexpected custom recipe: %#v", r)
	}
}

func TestValidateRejectsNegativeDiskRequirement(t *testing.T) {
	r := Recipe{
		Version: 1,
		ID: "test",
		Name: "Test",
		Runtime: Runtime{Engine: "sglang", Image: "example/image@sha256:abc"},
		Model: Model{Repository: "org/model", Revision: "0123456789012345678901234567890123456789"},
		Serve: Serve{ContextWindow: 1, MaxOutputTokens: 1, MaxRunningRequests: 1},
		Requirements: Requirements{MinimumDiskGB: -1},
	}
	if err := r.Validate(); err == nil {
		t.Fatal("expected negative disk requirement to be rejected")
	}
}
