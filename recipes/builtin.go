package recipes

import (
	_ "embed"

	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
)

//go:embed qwen-studio.toml
var qwenStudio []byte

//go:embed qwen-solo-rtx5090.toml
var qwenSoloRTX5090 []byte

//go:embed qwen-solo-dual.toml
var qwenSoloDual []byte

//go:embed qwen-solo-dual-max.toml
var qwenSoloDualMax []byte

//go:embed qwen-solo-uncensored.toml
var qwenSoloUncensored []byte

func QwenStudio() (recipe.Recipe, error) {
	return recipe.Parse(qwenStudio)
}

func QwenSoloRTX5090() (recipe.Recipe, error) {
	return recipe.Parse(qwenSoloRTX5090)
}

func QwenSoloDual() (recipe.Recipe, error) {
	return recipe.Parse(qwenSoloDual)
}

func QwenSoloDualMax() (recipe.Recipe, error) {
	return recipe.Parse(qwenSoloDualMax)
}

func Builtin() ([]recipe.Recipe, error) {
	studio, err := QwenStudio()
	if err != nil {
		return nil, err
	}
	solo, err := QwenSoloRTX5090()
	if err != nil {
		return nil, err
	}
	dual, err := QwenSoloDual()
	if err != nil {
		return nil, err
	}
	dualMax, err := QwenSoloDualMax()
	if err != nil {
		return nil, err
	}
	uncensored, err := recipe.Parse(qwenSoloUncensored)
	if err != nil {
		return nil, err
	}
	return []recipe.Recipe{studio, solo, dual, dualMax, uncensored}, nil
}
