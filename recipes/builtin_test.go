package recipes

import (
	"strings"
	"testing"

	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
)

func TestQwenStudioIsBundledAndValid(t *testing.T) {
	r, err := QwenStudio()
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "qwen-studio" || r.Kind != "text-generation" || r.Runtime.Engine != "sglang" || r.Profile.Status != "reference" || r.Requirements.GPUModel != "RTX PRO 6000 S" || r.Requirements.MinimumDiskGB != 120 {
		t.Fatalf("unexpected recipe: %#v", r)
	}
	assertDecisionMetadata(t, r)
}

func TestQwenSoloRTX5090IsBundledAndExperimental(t *testing.T) {
	r, err := QwenSoloRTX5090()
	if err != nil {
		t.Fatal(err)
	}
	cacheK, cacheV := r.LlamaCacheTypes()
	if r.ID != "qwen-solo-rtx5090" || r.Kind != "gguf-text-generation" || r.Runtime.Engine != "llama-cpp" || r.Profile.Status != "experimental" || r.Requirements.GPUModel != "RTX 5090" || r.Serve.ContextWindow != 262144 || r.Serve.MaxOutputTokens != 16384 || r.Serve.MaxRunningRequests != 1 || cacheK != "q8_0" || cacheV != "q8_0" {
		t.Fatalf("unexpected recipe: %#v", r)
	}
	assertDecisionMetadata(t, r)
}

func TestQwenSoloCapacityProfilesShareModelAndRuntimeIdentity(t *testing.T) {
	full, err := QwenSoloRTX5090()
	if err != nil {
		t.Fatal(err)
	}
	dual, err := QwenSoloDual()
	if err != nil {
		t.Fatal(err)
	}
	lab, err := QwenSoloDualMax()
	if err != nil {
		t.Fatal(err)
	}

	wantIdentity := [6]string{full.Model.Repository, full.Model.Revision, full.Model.Filename, full.Model.SHA256, full.Runtime.Image, full.Runtime.SourceRevision}
	for _, candidate := range []recipe.Recipe{dual, lab} {
		gotIdentity := [6]string{candidate.Model.Repository, candidate.Model.Revision, candidate.Model.Filename, candidate.Model.SHA256, candidate.Runtime.Image, candidate.Runtime.SourceRevision}
		if gotIdentity != wantIdentity {
			t.Fatalf("capacity profile changed model/runtime identity: got %q want %q", gotIdentity, wantIdentity)
		}
	}
	dualK, dualV := dual.LlamaCacheTypes()
	if dual.ID != "qwen-solo-dual" || dual.Profile.Status != "experimental" || dual.Serve.ContextWindow != 131072 || dual.Serve.MaxRunningRequests != 2 || dual.Serve.MaxOutputTokens != 16384 || dualK != "q8_0" || dualV != "q8_0" {
		t.Fatalf("unexpected Dual profile: %#v", dual)
	}
	labK, labV := lab.LlamaCacheTypes()
	if lab.ID != "qwen-solo-dual-max" || lab.Profile.Status != "lab" || lab.Serve.ContextWindow != 262144 || lab.Serve.MaxRunningRequests != 2 || lab.Serve.MaxOutputTokens != 16384 || labK != "q4_0" || labV != "q4_0" {
		t.Fatalf("unexpected Dual Max Lab profile: %#v", lab)
	}
}

func TestQwenSoloUncensoredContractIsUnchanged(t *testing.T) {
	all, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	r := all[4]
	k, v := r.LlamaCacheTypes()
	if r.ID != "qwen-solo-uncensored" || r.Profile.Status != "experimental" || r.Runtime.Engine != "llama-cpp" || r.Runtime.Image != "ghcr.io/maximiliengilet/sovereign-kit-llama@sha256:a9ceb65277f2f4dfd4cc3b5896f4398703e323249dd9bf87eb20b17b9f467239" || !r.Runtime.Precompiled || r.Runtime.SourceRevision != "86b351fd64d5ebbf1ba795ffd60c8f4a8c958613" || r.Model.Repository != "HauhauCS/Qwen3.8-27B-Uncensored-HauhauCS-Aggressive-MTP-GGUF" || r.Model.Revision != "993a5971fda8f30dd1b7eb2654792ba4415c7460" || r.Model.Filename != "Qwen3.8-27B-Uncensored-HauhauCS-Aggressive-Q4_K_P.gguf" || r.Model.SHA256 != "ba36dc3c2b2ff5e0aa5d71092a8894546996a6a119ae391803dda07cdc08516d" || r.Speculative == nil || r.Speculative.Algorithm != "draft-mtp" || r.Speculative.NumDraftTokens != 2 || r.Serve.ContextWindow != 65536 || r.Serve.MaxOutputTokens != 4096 || r.Serve.MaxRunningRequests != 4 || k != "q8_0" || v != "q8_0" {
		t.Fatalf("unexpected Uncensored contract: %#v", r)
	}
}

func TestBuiltinReturnsProfilesInStableOrder(t *testing.T) {
	builtin, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	if len(builtin) != 5 || builtin[0].ID != "qwen-studio" || builtin[1].ID != "qwen-solo-rtx5090" || builtin[2].ID != "qwen-solo-dual" || builtin[3].ID != "qwen-solo-dual-max" || builtin[4].ID != "qwen-solo-uncensored" {
		t.Fatalf("unexpected builtins: %#v", builtin)
	}
}

func assertDecisionMetadata(t *testing.T, r recipe.Recipe) {
	t.Helper()
	if r.Model.NativeContextWindow != 262144 || len(r.Profile.UseWhen) == 0 {
		t.Fatalf("missing decision metadata: %#v", r)
	}
	for _, item := range r.Profile.UseWhen {
		if strings.TrimSpace(item) == "" {
			t.Fatalf("blank use_when item: %#v", r.Profile.UseWhen)
		}
	}
}
