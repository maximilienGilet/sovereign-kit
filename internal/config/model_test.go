package config

import (
	"path/filepath"
	"testing"
)

func TestOptionalModelMetadataSurvivesSaveAndLoad(t *testing.T) {
	cfg := Studio("gpu.example", 22, "root", "/tmp/key", "/tmp/known")
	cfg.Model = Model{ID: "owner/solo", ContextWindow: 32768, MaxTokens: 4096}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || loaded.Model != cfg.Model {
		t.Fatalf("metadata lost: %#v %v", loaded.Model, err)
	}
}
