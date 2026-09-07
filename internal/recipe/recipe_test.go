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
	if r.Profile.Status != "reference" || r.Requirements.GPUModel != "RTX PRO 6000 S" || r.Requirements.GPUCount != 1 || !r.Requirements.StrictGPU {
		t.Fatalf("unexpected Studio profile: %#v", r)
	}
	if r.Requirements.MinimumVRAMGB != 96 || r.Requirements.MinimumDiskGB != 120 || r.Serve.ContextWindow != 262144 {
		t.Fatalf("unexpected recipe limits: %#v", r)
	}
}

func TestLoadQwenSoloCandidateRecipe(t *testing.T) {
	r, err := Load(filepath.Join("..", "..", "recipes", "qwen-solo-rtx5090.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "qwen-solo-rtx5090" || r.Profile.Status != "experimental" || r.Model.Repository != "unsloth/Qwen3.8-27B-GGUF" {
		t.Fatalf("unexpected Solo recipe: %#v", r)
	}
	if r.Requirements.GPUModel != "RTX 5090" || r.Requirements.GPUCount != 1 || !r.Requirements.StrictGPU || r.Requirements.MinimumVRAMGB != 32 || r.Requirements.MinimumDiskGB != 100 {
		t.Fatalf("unexpected Solo hardware: %#v", r.Requirements)
	}
	if r.Serve.ContextWindow != 262144 || r.Serve.MaxOutputTokens != 16384 || r.Serve.MaxRunningRequests != 1 {
		t.Fatalf("unexpected Solo limits: %#v", r.Serve)
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
		Version:      1,
		ID:           "test",
		Name:         "Test",
		Runtime:      Runtime{Engine: "sglang", Image: "example/image@sha256:abc"},
		Model:        Model{Repository: "org/model", Revision: "0123456789012345678901234567890123456789"},
		Serve:        Serve{ContextWindow: 1, MaxOutputTokens: 1, MaxRunningRequests: 1},
		Requirements: Requirements{MinimumDiskGB: -1},
	}
	if err := r.Validate(); err == nil {
		t.Fatal("expected negative disk requirement to be rejected")
	}
}

func TestValidateRejectsStrictGPUWithoutProfileStatus(t *testing.T) {
	r := Recipe{
		Version: 1,
		ID:      "strict",
		Name:    "Strict",
		Runtime: Runtime{Engine: "sglang", Image: "example/image@sha256:abc"},
		Model:   Model{Repository: "org/model", Revision: "0123456789012345678901234567890123456789"},
		Serve:   Serve{ContextWindow: 1, MaxOutputTokens: 1, MaxRunningRequests: 1},
		Requirements: Requirements{
			GPUModel:  "RTX 5090",
			GPUCount:  1,
			StrictGPU: true,
		},
	}
	if err := r.Validate(); err == nil {
		t.Fatal("expected strict GPU profile without status to be rejected")
	}
}

func TestValidateRejectsNativeContextBelowConfiguredContext(t *testing.T) {
	r := Recipe{
		Version: 1,
		ID:      "test",
		Name:    "Test",
		Kind:    "text-generation",
		Runtime: Runtime{Engine: "sglang", Image: "example/image@sha256:abc"},
		Model: Model{
			Repository:          "org/model",
			Revision:            "0123456789012345678901234567890123456789",
			NativeContextWindow: 16384,
		},
		Serve: Serve{ContextWindow: 32768, MaxOutputTokens: 1024, MaxRunningRequests: 1},
	}
	if err := r.Validate(); err == nil {
		t.Fatal("expected native context below configured context to be rejected")
	}
}

func TestValidateRejectsBlankUseWhenEntry(t *testing.T) {
	r := Recipe{
		Version: 1,
		ID:      "test",
		Name:    "Test",
		Kind:    "text-generation",
		Profile: Profile{UseWhen: []string{"valid", "  "}},
		Runtime: Runtime{Engine: "sglang", Image: "example/image@sha256:abc"},
		Model:   Model{Repository: "org/model", Revision: "0123456789012345678901234567890123456789"},
		Serve:   Serve{ContextWindow: 32768, MaxOutputTokens: 1024, MaxRunningRequests: 1},
	}
	if err := r.Validate(); err == nil {
		t.Fatal("expected blank use_when entry to be rejected")
	}
}

func TestCustomRecipeMayOmitNativeContextAndUseWhen(t *testing.T) {
	r, err := CustomHuggingFace("org/model", "319f741cce68d7914884900c138a1fbb70a42f30", false)
	if err != nil {
		t.Fatal(err)
	}
	if r.Model.NativeContextWindow != 0 || len(r.Profile.UseWhen) != 0 {
		t.Fatalf("unexpected custom decision metadata: %#v", r)
	}
}
