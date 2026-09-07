package catalogui

import (
	"testing"

	"github.com/maximilienGilet/sovereign-kit/recipes"
)

func TestEntryFromRecipePreservesTypedDecisionData(t *testing.T) {
	selectedRecipe, err := recipes.QwenSoloRTX5090()
	if err != nil {
		t.Fatal(err)
	}
	entry := EntryFromRecipe(selectedRecipe)
	if entry.Value != selectedRecipe.ID || entry.ContextWindow != 262144 || entry.NativeContextWindow != 262144 ||
		entry.MinimumVRAMGB != 32 || entry.GPUModel != "RTX 5090" || len(entry.UseWhen) != 3 || entry.KVCacheTypeK != "q8_0" || entry.KVCacheTypeV != "q8_0" {
		t.Fatalf("incomplete entry: %#v", entry)
	}
	selectedRecipe.Profile.UseWhen[0] = "mutated"
	if entry.UseWhen[0] == "mutated" {
		t.Fatal("entry aliases recipe use_when storage")
	}
}

func TestDefaultEntriesExposeDeploymentProfilesBeforeCustomFlow(t *testing.T) {
	entries := DefaultEntries()
	if len(entries) != 6 {
		t.Fatalf("entry count = %d", len(entries))
	}
	if entries[0].Name != "Qwen Studio" || entries[0].Status != "REFERENCE" || entries[0].MinimumVRAMGB != 96 || entries[0].ContextWindow != 262144 {
		t.Fatalf("unexpected Studio entry: %#v", entries[0])
	}
	if entries[1].Name != "Qwen Solo — Full Context" || entries[1].Status != "EXPERIMENTAL" || entries[1].Runtime != "llama-cpp" || entries[1].MinimumVRAMGB != 32 || entries[1].ContextWindow != 262144 || entries[1].MaxRunningRequests != 1 {
		t.Fatalf("unexpected Solo entry: %#v", entries[1])
	}
	if entries[2].Name != "Qwen Solo — Dual" || entries[2].ContextWindow != 131072 || entries[2].MaxRunningRequests != 2 || entries[2].KVCacheTypeK != "q8_0" {
		t.Fatalf("unexpected Dual entry: %#v", entries[2])
	}
	if entries[3].Name != "Qwen Solo — Dual Max Lab" || entries[3].Status != "LAB" || entries[3].ContextWindow != 262144 || entries[3].MaxRunningRequests != 2 || entries[3].KVCacheTypeK != "q4_0" {
		t.Fatalf("unexpected Dual Max entry: %#v", entries[3])
	}
	if entries[4].Name != "Qwen Solo Uncensored" || entries[4].Runtime != "llama-cpp" || entries[4].ContextWindow != 65536 {
		t.Fatalf("unexpected Uncensored entry: %#v", entries[4])
	}
	if entries[5].Name != "Custom Hugging Face" || entries[5].Status != "CUSTOM" {
		t.Fatalf("unexpected custom entry: %#v", entries[5])
	}
	for index, gpu := range []string{"RTX PRO 6000 S", "RTX 5090"} {
		if entries[index].GPUModel != gpu {
			t.Fatalf("entry %d GPU=%q", index, entries[index].GPUModel)
		}
	}
}
