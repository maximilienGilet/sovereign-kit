package catalogui

// DefaultEntries is the bundled, recipe-first starting catalog. Live provider
// offers and local measurements enrich these entries later; these labels do not
// claim that a particular current GPU has been benchmarked.
func DefaultEntries() []Entry {
	return []Entry{
		{
			Name: "Qwen Studio", Kind: "text-generation", VRAM: "96 GB", Context: "262K", Speed: "historical reference", Cost: "search offers", Status: "RECOMMENDED",
			Detail: Detail{
				Source: "RadixArk/Qwen3.8-27B-NVFP4", Confidence: "Historical reference only", Notes: "Select hardware, then measure this exact route before production",
			},
		},
		{
			Name: "Custom Hugging Face", Kind: "inspect first", VRAM: "unknown", Context: "unknown", Speed: "not measured", Cost: "depends on recipe", Status: "CUSTOM",
			Detail: Detail{
				Source: "Enter an owner/model repository", Confidence: "No automatic launch", Notes: "Resolve a revision, classify the artifact, then create a recipe",
			},
		},
	}
}
