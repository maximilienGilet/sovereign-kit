package setup

import (
	"context"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"testing"
)

func TestSuccessfulDeploymentPersistsActualRecipeModelLimits(t *testing.T) {
	_, _, _, _, _, _, deps := successfulSetup()
	var saved config.Config
	deps.SaveConfig = func(_ string, c config.Config) error { saved = c; return nil }
	if _, err := RunVast(context.Background(), "token", validRecipe(), testOptions(), deps); err != nil {
		t.Fatal(err)
	}
	if saved.Model.ID != "Qwen/Qwen" || saved.Model.ContextWindow != 128 || saved.Model.MaxTokens != 32 {
		t.Fatalf("missing deployed model metadata: %#v", saved.Model)
	}
}
