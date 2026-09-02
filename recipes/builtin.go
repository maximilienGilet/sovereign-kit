package recipes

import (
	_ "embed"

	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
)

//go:embed qwen-studio.toml
var qwenStudio []byte

func QwenStudio() (recipe.Recipe, error) {
	return recipe.Parse(qwenStudio)
}
