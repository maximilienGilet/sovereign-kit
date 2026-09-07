// Package recipe owns the small set of supported server recipes.
package recipe

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

var gitRevision = regexp.MustCompile(`^[a-f0-9]{40}$`)

type Recipe struct {
	Version      int          `toml:"version"`
	ID           string       `toml:"id"`
	Name         string       `toml:"name"`
	Kind         string       `toml:"kind"`
	Profile      Profile      `toml:"profile"`
	Runtime      Runtime      `toml:"runtime"`
	Model        Model        `toml:"model"`
	Speculative  *Speculative `toml:"speculative"`
	Serve        Serve        `toml:"serve"`
	Requirements Requirements `toml:"requirements"`
}

type Profile struct {
	Status   string   `toml:"status"`
	Summary  string   `toml:"summary"`
	Evidence string   `toml:"evidence"`
	UseWhen  []string `toml:"use_when"`
}

type Speculative struct {
	Algorithm       string `toml:"algorithm"`
	DraftRepository string `toml:"draft_repository"`
	DraftRevision   string `toml:"draft_revision"`
	NumDraftTokens  int    `toml:"num_draft_tokens"`
}

type Runtime struct {
	Precompiled            bool    `toml:"precompiled" json:",omitempty"`
	SourceRevision         string  `toml:"source_revision" json:",omitempty"`
	Engine                 string  `toml:"engine"`
	Image                  string  `toml:"image"`
	Quantization           string  `toml:"quantization"`
	KVCacheDType           string  `toml:"kv_cache_dtype"`
	KVCacheTypeK           string  `toml:"kv_cache_type_k" json:",omitempty"`
	KVCacheTypeV           string  `toml:"kv_cache_type_v" json:",omitempty"`
	GPUMemoryUtilization   float64 `toml:"gpu_memory_utilization"`
	DisableAsyncScheduling bool    `toml:"disable_async_scheduling"`
}

type Model struct {
	SHA256              string `toml:"sha256" json:",omitempty"`
	Repository          string `toml:"repository"`
	Revision            string `toml:"revision"`
	Filename            string `toml:"filename"`
	TrustRemoteCode     bool   `toml:"trust_remote_code"`
	NativeContextWindow int    `toml:"native_context_window"`
}

type Serve struct {
	ContextWindow      int `toml:"context_window"`
	MaxOutputTokens    int `toml:"max_output_tokens"`
	MaxRunningRequests int `toml:"max_running_requests"`
}

type Requirements struct {
	GPUModel      string `toml:"gpu_model"`
	GPUCount      int    `toml:"gpu_count"`
	StrictGPU     bool   `toml:"strict_gpu"`
	MinimumVRAMGB int    `toml:"minimum_vram_gb"`
	MinimumDiskGB int    `toml:"minimum_disk_gb"`
}

func Load(path string) (Recipe, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Recipe{}, err
	}
	return Parse(contents)
}

func Parse(contents []byte) (Recipe, error) {
	var value Recipe
	if err := toml.Unmarshal(contents, &value); err != nil {
		return Recipe{}, fmt.Errorf("parse recipe: %w", err)
	}
	if err := value.Validate(); err != nil {
		return Recipe{}, err
	}
	return value, nil
}

// CustomHuggingFace creates a conservative SGLang recipe. It makes no capacity
// claim; the operator must measure it before production use.
func CustomHuggingFace(repository, revision string, trustRemoteCode bool) (Recipe, error) {
	recipe := Recipe{
		Version: 1,
		ID:      "custom-huggingface",
		Name:    "Custom Hugging Face model",
		Kind:    "text-generation",
		Runtime: Runtime{Engine: "sglang", Image: "lmsysorg/sglang@sha256:e21dd539b36ea7842101393ec3fe3b0d453626cd8251e4d4db33af0cd97d7f0b"},
		Model:   Model{Repository: repository, Revision: revision, TrustRemoteCode: trustRemoteCode},
		Serve:   Serve{ContextWindow: 32768, MaxOutputTokens: 4096, MaxRunningRequests: 1},
	}
	return recipe, recipe.Validate()
}

// LlamaCacheTypes resolves the llama.cpp cache formats while preserving the
// q8_0 behavior of recipes and checkpoints created before these fields existed.
func (recipe Recipe) LlamaCacheTypes() (string, string) {
	k, v := recipe.Runtime.KVCacheTypeK, recipe.Runtime.KVCacheTypeV
	if k == "" {
		k = "q8_0"
	}
	if v == "" {
		v = "q8_0"
	}
	return k, v
}

func (recipe Recipe) Validate() error {
	if recipe.Version != 1 || strings.TrimSpace(recipe.ID) == "" || strings.TrimSpace(recipe.Name) == "" {
		return fmt.Errorf("recipe version, id, and name are required")
	}
	if recipe.Profile.Status != "" && recipe.Profile.Status != "reference" && recipe.Profile.Status != "experimental" && recipe.Profile.Status != "lab" {
		return fmt.Errorf("recipe profile status must be reference, experimental, or lab")
	}
	if recipe.Requirements.StrictGPU && recipe.Profile.Status == "" {
		return fmt.Errorf("strict GPU requirements need a profile status")
	}
	if recipe.Requirements.StrictGPU &&
		(strings.TrimSpace(recipe.Requirements.GPUModel) == "" || recipe.Requirements.GPUCount < 1) {
		return fmt.Errorf("strict GPU requirements need a model and positive GPU count")
	}
	if recipe.Requirements.GPUCount < 0 {
		return fmt.Errorf("GPU count cannot be negative")
	}
	if !strings.Contains(recipe.Runtime.Image, "@sha256:") {
		return fmt.Errorf("recipe runtime image must be pinned by digest")
	}
	if strings.TrimSpace(recipe.Model.Repository) == "" || !strings.Contains(recipe.Model.Repository, "/") {
		return fmt.Errorf("model repository must be an owner/name Hugging Face repository")
	}
	if !gitRevision.MatchString(recipe.Model.Revision) {
		return fmt.Errorf("model revision must be a pinned 40-character Git commit")
	}
	switch recipe.Kind {
	case "", "text-generation":
		if recipe.Runtime.Engine != "sglang" {
			return fmt.Errorf("text-generation recipes require SGLang")
		}
	case "speculative-text-generation":
		switch recipe.Runtime.Engine {
		case "sglang":
			if recipe.Speculative == nil || recipe.Speculative.Algorithm != "dflash" ||
				!strings.Contains(recipe.Speculative.DraftRepository, "/") ||
				!gitRevision.MatchString(recipe.Speculative.DraftRevision) || recipe.Speculative.NumDraftTokens < 1 {
				return fmt.Errorf("DFlash recipes require a pinned draft model and positive draft token count")
			}
		case "vllm":
			if recipe.Runtime.Quantization != "modelopt" ||
				recipe.Runtime.KVCacheDType != "turboquant_4bit_nc" ||
				recipe.Runtime.GPUMemoryUtilization <= 0 || recipe.Runtime.GPUMemoryUtilization >= 1 ||
				!recipe.Runtime.DisableAsyncScheduling ||
				recipe.Speculative == nil || recipe.Speculative.Algorithm != "qwen3_5_mtp" ||
				recipe.Speculative.NumDraftTokens != 4 ||
				strings.TrimSpace(recipe.Speculative.DraftRepository) != "" ||
				strings.TrimSpace(recipe.Speculative.DraftRevision) != "" {
				return fmt.Errorf("vLLM TurboQuant MTP recipes require the reviewed single-GPU contract")
			}
		default:
			return fmt.Errorf("speculative recipes require SGLang or vLLM")
		}
	case "gguf-text-generation":
		if recipe.Runtime.Engine != "llama-cpp" || !strings.HasSuffix(recipe.Model.Filename, ".gguf") {
			return fmt.Errorf("GGUF recipes require llama.cpp and a selected .gguf filename")
		}
		cacheK, cacheV := recipe.LlamaCacheTypes()
		for _, cacheType := range []string{cacheK, cacheV} {
			if cacheType != "q8_0" && cacheType != "q4_0" {
				return fmt.Errorf("llama.cpp KV cache types must be q8_0 or q4_0")
			}
		}
		if recipe.Speculative != nil {
			if recipe.Speculative.Algorithm != "draft-mtp" || recipe.Speculative.NumDraftTokens != 2 || recipe.Speculative.DraftRepository != "" || recipe.Speculative.DraftRevision != "" ||
				!gitRevision.MatchString(recipe.Runtime.SourceRevision) || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(recipe.Model.SHA256) ||
				!regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9_.-]*/[A-Za-z0-9_-][A-Za-z0-9_.-]*$`).MatchString(recipe.Model.Repository) ||
				!regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9_.-]*\.gguf$`).MatchString(recipe.Model.Filename) {
				return fmt.Errorf("native llama.cpp MTP requires pinned source, model SHA256, safe filename and repository, depth two")
			}
		}
		// ContextWindow is the advertised per-request limit, not the shared KV pool.
		if recipe.Serve.MaxRunningRequests < 1 || recipe.Serve.MaxRunningRequests > 4 ||
			recipe.Serve.ContextWindow > 262144 ||
			recipe.Serve.ContextWindow > 524288/recipe.Serve.MaxRunningRequests ||
			recipe.Serve.MaxOutputTokens > recipe.Serve.ContextWindow {
			return fmt.Errorf("llama.cpp requires 1–4 slots, at most 262144 context tokens per slot and 524288 total, and output within per-slot context")
		}
	default:
		return fmt.Errorf("unsupported recipe kind %q", recipe.Kind)
	}
	if recipe.Serve.ContextWindow < 1 || recipe.Serve.MaxOutputTokens < 1 || recipe.Serve.MaxRunningRequests < 1 {
		return fmt.Errorf("positive server limits are required")
	}
	if recipe.Model.NativeContextWindow < 0 ||
		(recipe.Model.NativeContextWindow > 0 && recipe.Model.NativeContextWindow < recipe.Serve.ContextWindow) {
		return fmt.Errorf("native context must contain the configured context window")
	}
	for _, item := range recipe.Profile.UseWhen {
		if strings.TrimSpace(item) == "" {
			return fmt.Errorf("recipe use_when entries cannot be blank")
		}
	}
	if recipe.Requirements.MinimumVRAMGB < 0 || recipe.Requirements.MinimumDiskGB < 0 {
		return fmt.Errorf("minimum VRAM and disk cannot be negative")
	}
	return nil
}
