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
	if r.Requirements.MinimumVRAMGB != 96 || r.Serve.ContextWindow != 262144 {
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
