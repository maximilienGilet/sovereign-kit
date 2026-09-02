// Package catalog classifies Hugging Face artifacts before a recipe is created.
package catalog

import (
	"strings"
)

type Status string

const (
	Supported         Status = "supported"
	NeedsRecipe       Status = "needs-recipe"
	NeedsConfirmation Status = "needs-confirmation"
	Unsupported       Status = "unsupported"
)

type Artifact struct {
	Repository         string
	PipelineTag        string
	Tags               []string
	Files              []string
	RequiresRemoteCode bool
}

type Result struct {
	Kind           string
	Engine         string
	Status         Status
	RequiresTarget bool
	Reason         string
}

// Classify is deliberately conservative. It recommends an adapter only for
// recognized artifacts and never turns remote code into an automatic launch.
func Classify(artifact Artifact) Result {
	if hasTag(artifact.Tags, "draft-model") || hasTag(artifact.Tags, "speculative-decoding") {
		return Result{Kind: "speculative-text-generation", Engine: "sglang", Status: NeedsRecipe, RequiresTarget: true, Reason: "Draft model requires a pinned target and speculative recipe"}
	}
	if hasGGUF(artifact.Files) {
		return Result{Kind: "gguf-text-generation", Engine: "llama-cpp", Status: Supported, Reason: "GGUF artifact detected"}
	}
	if artifact.RequiresRemoteCode {
		return Result{Kind: "text-generation", Engine: "sglang", Status: NeedsConfirmation, Reason: "Remote code requires explicit operator approval"}
	}
	switch strings.ToLower(artifact.PipelineTag) {
	case "feature-extraction", "sentence-similarity":
		return Result{Kind: "embeddings", Status: NeedsRecipe, Reason: "Embedding endpoint adapter is not implemented yet"}
	case "image-text-to-text", "image-to-text", "visual-question-answering":
		return Result{Kind: "multimodal", Status: NeedsRecipe, Reason: "Multimodal request and media handling require a dedicated recipe"}
	case "text-generation", "":
		return Result{Kind: "text-generation", Engine: "sglang", Status: Supported, Reason: "Standard text-generation candidate"}
	default:
		return Result{Kind: "unknown", Status: Unsupported, Reason: "No supported runtime adapter for this artifact"}
	}
}

func hasTag(tags []string, wanted string) bool {
	for _, tag := range tags {
		if strings.EqualFold(tag, wanted) {
			return true
		}
	}
	return false
}

func hasGGUF(files []string) bool {
	for _, file := range files {
		if strings.HasSuffix(strings.ToLower(file), ".gguf") {
			return true
		}
	}
	return false
}
