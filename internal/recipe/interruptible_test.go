package recipe

import (
	"path/filepath"
	"testing"
)

func TestAllowInterruptibleDefaultsFalseAndParses(t *testing.T) {
	r, err := Load(filepath.Join("..", "..", "recipes", "qwen-solo-rtx5090.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Requirements.AllowInterruptible {
		t.Fatal("builtin Solo recipe must forbid interruptible offers by default")
	}
	parsed, err := Parse([]byte(`version = 1
id = "interruptible-lab"
name = "Interruptible lab"
kind = "text-generation"

[runtime]
engine = "sglang"
image = "lmsysorg/sglang@sha256:e21dd539b36ea7842101393ec3fe3b0d453626cd8251e4d4db33af0cd97d7f0b"

[model]
repository = "RadixArk/Qwen3.8-27B-NVFP4"
revision = "319f741cce68d7914884900c138a1fbb70a42f30"

[serve]
context_window = 32768
max_output_tokens = 4096
max_running_requests = 1

[requirements]
minimum_vram_gb = 96
allow_interruptible = true
`))
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.Requirements.AllowInterruptible {
		t.Fatal("allow_interruptible = true was not parsed")
	}
}
