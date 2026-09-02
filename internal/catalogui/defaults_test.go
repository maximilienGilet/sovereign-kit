package catalogui

import "testing"

func TestDefaultEntriesPutLaunchableRecipeBeforeCustomFlow(t *testing.T) {
	entries := DefaultEntries()
	if len(entries) < 2 {
		t.Fatalf("entry count = %d", len(entries))
	}
	if entries[0].Name != "Qwen Studio" || entries[0].Status != "RECOMMENDED" {
		t.Fatalf("unexpected first entry: %#v", entries[0])
	}
	if entries[1].Name != "Custom Hugging Face" || entries[1].Status != "CUSTOM" {
		t.Fatalf("unexpected custom entry: %#v", entries[1])
	}
}
