package catalogui

import (
	"fmt"
	"strings"

	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/recipes"
)

func DefaultEntries() []Entry {
	builtin, err := recipes.Builtin()
	if err != nil {
		panic(fmt.Sprintf("load built-in recipes: %v", err))
	}
	entries := make([]Entry, 0, len(builtin)+1)
	for _, selectedRecipe := range builtin {
		entries = append(entries, EntryFromRecipe(selectedRecipe))
	}
	return append(entries, CustomEntry())
}

func EntryFromRecipe(selectedRecipe recipe.Recipe) Entry {
	status := strings.ToUpper(strings.TrimSpace(selectedRecipe.Profile.Status))
	if status == "" {
		status = "UNKNOWN/UNMEASURED"
	}
	cacheK, cacheV := "", ""
	if selectedRecipe.Runtime.Engine == "llama-cpp" {
		cacheK, cacheV = selectedRecipe.LlamaCacheTypes()
	}
	return Entry{
		Value:               selectedRecipe.ID,
		Name:                selectedRecipe.Name,
		Kind:                selectedRecipe.Kind,
		Status:              status,
		Summary:             selectedRecipe.Profile.Summary,
		Evidence:            selectedRecipe.Profile.Evidence,
		UseWhen:             append([]string(nil), selectedRecipe.Profile.UseWhen...),
		ModelRepository:     selectedRecipe.Model.Repository,
		ModelRevision:       selectedRecipe.Model.Revision,
		Runtime:             selectedRecipe.Runtime.Engine,
		GPUModel:            selectedRecipe.Requirements.GPUModel,
		GPUCount:            selectedRecipe.Requirements.GPUCount,
		StrictGPU:           selectedRecipe.Requirements.StrictGPU,
		ContextWindow:       selectedRecipe.Serve.ContextWindow,
		NativeContextWindow: selectedRecipe.Model.NativeContextWindow,
		MaxOutputTokens:     selectedRecipe.Serve.MaxOutputTokens,
		MaxRunningRequests:  selectedRecipe.Serve.MaxRunningRequests,
		KVCacheTypeK:        cacheK,
		KVCacheTypeV:        cacheV,
		MinimumVRAMGB:       selectedRecipe.Requirements.MinimumVRAMGB,
		MinimumDiskGB:       selectedRecipe.Requirements.MinimumDiskGB,
	}
}

func CustomEntry() Entry {
	return Entry{
		Value:           "custom",
		Name:            "Custom Hugging Face",
		Kind:            "inspect first",
		Status:          "CUSTOM",
		Summary:         "Choose a model first; performance is not measured",
		Evidence:        "No automatic launch evidence",
		ModelRepository: "Enter an owner/model repository",
		GPUModel:        "Resolve a revision, classify the artifact, then create a recipe",
		Custom:          true,
	}
}

func formatCatalogCount(value int) string {
	digits := fmt.Sprintf("%d", value)
	var formatted strings.Builder
	for index, digit := range digits {
		if index > 0 && (len(digits)-index)%3 == 0 {
			formatted.WriteByte(',')
		}
		formatted.WriteRune(digit)
	}
	return formatted.String()
}
