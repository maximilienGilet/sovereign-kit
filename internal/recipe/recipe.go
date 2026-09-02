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
	Runtime      Runtime      `toml:"runtime"`
	Model        Model        `toml:"model"`
	Speculative  *Speculative `toml:"speculative"`
	Serve        Serve        `toml:"serve"`
	Requirements Requirements `toml:"requirements"`
}

type Speculative struct {
	Algorithm       string `toml:"algorithm"`
	DraftRepository string `toml:"draft_repository"`
	DraftRevision   string `toml:"draft_revision"`
	NumDraftTokens  int    `toml:"num_draft_tokens"`
}

type Runtime struct {
	Engine string `toml:"engine"`
	Image  string `toml:"image"`
}

type Model struct {
	Repository      string `toml:"repository"`
	Revision        string `toml:"revision"`
	Filename        string `toml:"filename"`
	TrustRemoteCode bool   `toml:"trust_remote_code"`
}

type Serve struct {
	ContextWindow      int `toml:"context_window"`
	MaxOutputTokens    int `toml:"max_output_tokens"`
	MaxRunningRequests int `toml:"max_running_requests"`
}

type Requirements struct {
	MinimumVRAMGB int `toml:"minimum_vram_gb"`
}

func Load(path string) (Recipe, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Recipe{}, err
	}
	var recipe Recipe
	if err := toml.Unmarshal(contents, &recipe); err != nil {
		return Recipe{}, fmt.Errorf("parse recipe: %w", err)
	}
	if err := recipe.Validate(); err != nil {
		return Recipe{}, err
	}
	return recipe, nil
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

func (recipe Recipe) Validate() error {
	if recipe.Version != 1 || strings.TrimSpace(recipe.ID) == "" || strings.TrimSpace(recipe.Name) == "" {
		return fmt.Errorf("recipe version, id, and name are required")
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
		if recipe.Runtime.Engine != "sglang" {
			return fmt.Errorf("speculative recipes require SGLang")
		}
		if recipe.Speculative == nil || recipe.Speculative.Algorithm != "dflash" ||
			!strings.Contains(recipe.Speculative.DraftRepository, "/") ||
			!gitRevision.MatchString(recipe.Speculative.DraftRevision) || recipe.Speculative.NumDraftTokens < 1 {
			return fmt.Errorf("DFlash recipes require a pinned draft model and positive draft token count")
		}
	case "gguf-text-generation":
		if recipe.Runtime.Engine != "llama-cpp" || !strings.HasSuffix(recipe.Model.Filename, ".gguf") {
			return fmt.Errorf("GGUF recipes require llama.cpp and a selected .gguf filename")
		}
	default:
		return fmt.Errorf("unsupported recipe kind %q", recipe.Kind)
	}
	if recipe.Serve.ContextWindow < 1 || recipe.Serve.MaxOutputTokens < 1 || recipe.Serve.MaxRunningRequests < 1 {
		return fmt.Errorf("positive server limits are required")
	}
	if recipe.Requirements.MinimumVRAMGB < 0 {
		return fmt.Errorf("minimum VRAM cannot be negative")
	}
	return nil
}
