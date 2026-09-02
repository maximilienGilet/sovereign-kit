package recipes

import "testing"

func TestQwenStudioIsBundledAndValid(t *testing.T) {
	r, err := QwenStudio()
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "qwen-studio" || r.Runtime.Engine != "sglang" || r.Requirements.MinimumDiskGB != 120 {
		t.Fatalf("unexpected recipe: %#v", r)
	}
}
