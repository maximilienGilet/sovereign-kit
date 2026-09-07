package recipe

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPrecompiledRuntimePreservesLegacySnapshots(t *testing.T) {
	legacy, err := Parse([]byte(llamaFixture))
	if err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(before), "Precompiled") {
		t.Fatal("new default field changed legacy fingerprint payload")
	}
	compiled, err := Parse([]byte(strings.Replace(llamaFixture, "[runtime]", "[runtime]\nprecompiled=true", 1)))
	if err != nil || !compiled.Runtime.Precompiled {
		t.Fatalf("precompiled recipe did not opt in: %v", err)
	}
	payload, err := json.Marshal(compiled)
	if err != nil {
		t.Fatal(err)
	}
	var restored Recipe
	if err := json.Unmarshal(payload, &restored); err != nil || !restored.Runtime.Precompiled {
		t.Fatalf("checkpoint lost precompiled mode: %v", err)
	}
	// Checkpoints decode into a fresh recipe; missing opt-in must remain false.
	var old Recipe
	if err := json.Unmarshal(before, &old); err != nil || old.Runtime.Precompiled {
		t.Fatalf("legacy snapshot silently upgraded: %v", err)
	}
	if k, v := old.LlamaCacheTypes(); k != "q8_0" || v != "q8_0" {
		t.Fatalf("legacy checkpoint cache defaults = %q/%q", k, v)
	}
}

const llamaFixture = `version=1
id="qwen-solo"
name="Qwen Solo"
kind="gguf-text-generation"
[runtime]
engine="llama-cpp"
image="nvidia/cuda@sha256:4ff859525f99de5782aa73607ce24219b07dddd48d12b97c1c301d7e1cfb0a87"
source_revision="86b351fd64d5ebbf1ba795ffd60c8f4a8c958613"
[model]
repository="owner/model"
revision="993a5971fda8f30dd1b7eb2654792ba4415c7460"
filename="model.gguf"
sha256="ba36dc3c2b2ff5e0aa5d71092a8894546996a6a119ae391803dda07cdc08516d"
[speculative]
algorithm="draft-mtp"
num_draft_tokens=2
[serve]
context_window=262144
max_output_tokens=4096
max_running_requests=1
`

func TestNativeLlamaParallelContextBudget(t *testing.T) {
	for _, tc := range []struct {
		slots, context, output int
		valid                  bool
	}{
		{1, 262144, 4096, true}, {2, 131072, 4096, true}, {4, 65536, 16384, true},
		{2, 262144, 16384, true}, {2, 262145, 16384, false},
		{4, 262144, 4096, false}, {5, 32768, 4096, false}, {0, 65536, 4096, false}, {4, 65536, 65537, false},
	} {
		r, err := Parse([]byte(llamaFixture))
		if err != nil {
			t.Fatal(err)
		}
		r.Serve.MaxRunningRequests = tc.slots
		r.Serve.ContextWindow = tc.context
		r.Serve.MaxOutputTokens = tc.output
		if err := r.Validate(); (err == nil) != tc.valid {
			t.Errorf("slots=%d context=%d output=%d: %v", tc.slots, tc.context, tc.output, err)
		}
	}
}

func TestLlamaCacheTypesDefaultAndValidate(t *testing.T) {
	r, err := Parse([]byte(llamaFixture))
	if err != nil {
		t.Fatal(err)
	}
	if k, v := r.LlamaCacheTypes(); k != "q8_0" || v != "q8_0" {
		t.Fatalf("legacy defaults = %q/%q", k, v)
	}

	explicit := strings.Replace(llamaFixture, "[runtime]", "[runtime]\nkv_cache_type_k=\"q4_0\"\nkv_cache_type_v=\"q4_0\"", 1)
	r, err = Parse([]byte(explicit))
	if err != nil {
		t.Fatal(err)
	}
	if k, v := r.LlamaCacheTypes(); k != "q4_0" || v != "q4_0" {
		t.Fatalf("explicit cache types = %q/%q", k, v)
	}

	invalid := strings.Replace(llamaFixture, "[runtime]", "[runtime]\nkv_cache_type_k=\"f32\"", 1)
	if _, err := Parse([]byte(invalid)); err == nil {
		t.Fatal("unsupported llama.cpp cache type accepted")
	}
}

func TestNativeLlamaMTPRejectsUnpinnedOrUnsafeInputs(t *testing.T) {
	if _, err := Parse([]byte(llamaFixture)); err != nil {
		t.Fatal(err)
	}
	for _, change := range [][2]string{
		{`source_revision="86b351fd64d5ebbf1ba795ffd60c8f4a8c958613"`, `source_revision="main"`},
		{`sha256="ba36dc3c2b2ff5e0aa5d71092a8894546996a6a119ae391803dda07cdc08516d"`, `sha256=""`},
		{`filename="model.gguf"`, `filename="../model.gguf"`},
		{`algorithm="draft-mtp"`, `algorithm="dflash"`},
		{`num_draft_tokens=2`, `num_draft_tokens=0`},
		{`repository="owner/model"`, `repository="../model"`},
	} {
		if _, err := Parse([]byte(strings.Replace(llamaFixture, change[0], change[1], 1))); err == nil {
			t.Errorf("accepted unsafe input: %s", change[1])
		}
	}
}
