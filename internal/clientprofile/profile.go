package clientprofile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
)

type Integration string

const (
	Pi       Integration = "Pi / Oh My Pi"
	OpenCode Integration = "OpenCode"
)

type State string

const (
	Absent     State = "absent"
	Ready      State = "ready"
	Incomplete State = "incomplete"
	Different  State = "different"
	Unreadable State = "unreadable"
)

type Target struct {
	Kind Integration
	Path string
}
type Inspection struct {
	State                                State
	Detail, Command, Fingerprint, Backup string
	Paths                                []string
}
type Service struct {
	Home     string
	Getenv   func(string) string
	LookPath func(string) (string, error)
	Run      func(context.Context, string, []string, []string) error
}

const provider = "sovereign-qwen"

var packages = []string{"npm:pi-subagents@0.62.0", "npm:oh-my-pi@0.2.0"}

func (s Service) Resolve(kind Integration) (Target, error) {
	home := s.Home
	var err error
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return Target{}, err
		}
	}
	home, err = filepath.EvalSymlinks(home)
	if err != nil {
		return Target{}, err
	}
	getenv := s.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	path := ""
	switch kind {
	case Pi:
		path = getenv("PI_SOVEREIGN_DIR")
		if path == "" {
			path = getenv("PI_CODING_AGENT_DIR")
		}
		if path == "" {
			path = filepath.Join(home, ".pi/profiles/sovereign/agent")
		}
	case OpenCode:
		path = getenv("SOVEREIGN_OPENCODE_CONFIG")
		if path == "" {
			path = filepath.Join(home, ".config/opencode/sovereign.json")
		}
	default:
		return Target{}, fmt.Errorf("unsupported integration")
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) == "/" || filepath.Clean(path) == home {
		return Target{}, fmt.Errorf("profile must have a dedicated absolute path")
	}
	path = filepath.Clean(path)
	if path == filepath.Join(home, ".pi/agent") || path == filepath.Join(home, ".config/opencode/opencode.json") || path == filepath.Join(home, ".config/opencode/opencode.jsonc") {
		return Target{}, fmt.Errorf("refusing global user profile: %s", path)
	}
	if err = safePath(path); err != nil {
		return Target{}, err
	}
	return Target{kind, path}, nil
}
func (s Service) lookup(name string) error {
	f := s.LookPath
	if f == nil {
		f = exec.LookPath
	}
	_, err := f(name)
	return err
}
func managed(t Target) []string {
	if t.Kind == Pi {
		return []string{"models.json", "settings.json", "npm"}
	}
	return []string{filepath.Base(t.Path)}
}
func root(t Target) string {
	if t.Kind == Pi {
		return t.Path
	}
	return filepath.Dir(t.Path)
}
func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
func command(t Target) string {
	if t.Kind == Pi {
		return "PI_CODING_AGENT_DIR=" + quote(t.Path) + " pi"
	}
	return "OPENCODE_CONFIG_CONTENT=\"$(cat " + quote(t.Path) + ")\" QWEN_LOCAL_API_KEY=local-qwen-tunnel opencode"
}

func (s Service) Inspect(ctx context.Context, t Target, e Endpoint) Inspection {
	result := Inspection{State: Incomplete}
	if t.Kind != Pi && t.Kind != OpenCode {
		result.State = Unreadable
		result.Detail = "Unsupported integration"
		return result
	}
	for _, name := range managed(t) {
		result.Paths = append(result.Paths, filepath.Join(root(t), name))
	}
	fail := func(err error) Inspection { result.State = Unreadable; result.Detail = err.Error(); return result }
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if err := safePath(t.Path); err != nil {
		return fail(err)
	}
	fingerprint, err := snapshot(ctx, t)
	if err != nil {
		return fail(err)
	}
	result.Fingerprint = fingerprint
	if _, err := os.Lstat(t.Path); os.IsNotExist(err) {
		result.State = Absent
		result.Detail = "No profile at this destination"
		return result
	} else if err != nil {
		return fail(err)
	}
	maps := map[string]map[string]any{}
	names := managed(t)
	if t.Kind == Pi {
		names = names[:2]
	}
	missing := false
	for _, name := range names {
		v, err := readObject(filepath.Join(root(t), name))
		if os.IsNotExist(err) {
			missing = true
			continue
		}
		if err != nil {
			return fail(err)
		}
		maps[name] = v
	}
	if missing {
		result.Detail = "Required configuration files are missing"
		return result
	}
	if !e.Installable() {
		result.Detail = "Cannot verify compatibility without model identity and verified limits"
		return result
	}
	expected, err := render(t, e, maps)
	if err != nil {
		return fail(err)
	}
	for name, want := range expected {
		if !reflect.DeepEqual(maps[name], want) {
			result.State = Different
			result.Detail = "Endpoint, model, defaults, limits or integration settings differ"
			return result
		}
	}
	if t.Kind == Pi {
		if err := s.lookup("pi"); err != nil {
			result.Detail = "Pi CLI is missing; install Pi separately first"
			return result
		}
		if err := checkPackages(t.Path); err != nil {
			result.Detail = err.Error()
			return result
		}
	} else if err := s.lookup("opencode"); err != nil {
		result.Detail = "OpenCode CLI is missing (pinned installer: opencode-ai@1.18.25)"
		return result
	}
	result.State = Ready
	result.Detail = "Configuration and required dependencies verified"
	result.Command = command(t)
	return result
}
func readObject(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v map[string]any
	if err = json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("unreadable %s: %w", path, err)
	}
	if v == nil {
		return nil, fmt.Errorf("expected JSON object: %s", path)
	}
	return v, nil
}
func object(v map[string]any, key string) (map[string]any, error) {
	if existing, ok := v[key]; ok {
		m, ok := existing.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s must be an object", key)
		}
		return m, nil
	}
	m := map[string]any{}
	v[key] = m
	return m, nil
}
func render(t Target, e Endpoint, original map[string]map[string]any) (map[string]map[string]any, error) {
	// Clone parsed objects so inspection remains a pure comparison.
	raw, _ := json.Marshal(original)
	out := map[string]map[string]any{}
	json.Unmarshal(raw, &out)
	get := func(name string) map[string]any {
		if out[name] == nil {
			out[name] = map[string]any{}
		}
		return out[name]
	}
	if t.Kind == Pi {
		settings := get("settings.json")
		settings["defaultProvider"] = provider
		settings["defaultModel"] = e.ID
		list, ok := settings["packages"].([]any)
		if settings["packages"] != nil && !ok {
			return nil, fmt.Errorf("packages must be an array")
		}
		for _, required := range packages {
			found := false
			for i, item := range list {
				source, _ := item.(string)
				if obj, ok := item.(map[string]any); ok {
					source, _ = obj["source"].(string)
				}
				name := strings.Split(required, "@")[0]
				if source == name || strings.HasPrefix(source, name+"@") {
					if entry, ok := item.(map[string]any); ok {
						entry["source"] = required
					} else {
						list[i] = required
					}
					found = true
				}
			}
			if !found {
				list = append(list, required)
			}
		}
		settings["packages"] = list
		sub, err := object(settings, "subagents")
		if err != nil {
			return nil, err
		}
		sub["defaultModel"] = provider + "/" + e.ID
		scope, err := object(sub, "modelScope")
		if err != nil {
			return nil, err
		}
		mergeOwned(scope, map[string]any{"enforce": true, "strict": true, "allow": []any{provider + "/*"}})
		models := get("models.json")
		providers, err := object(models, "providers")
		if err != nil {
			return nil, err
		}
		p, err := object(providers, provider)
		if err != nil {
			return nil, err
		}
		mergeOwned(p, map[string]any{"baseUrl": e.BaseURL, "api": "openai-completions", "apiKey": "local-qwen-tunnel", "compat": map[string]any{"supportsDeveloperRole": false, "supportsReasoningEffort": false}})
		list, ok = p["models"].([]any)
		if p["models"] != nil && !ok {
			return nil, fmt.Errorf("provider models must be an array")
		}
		var selected map[string]any
		for _, entry := range list {
			model, ok := entry.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("provider model must be an object")
			}
			if model["id"] == e.ID {
				selected = model
				break
			}
		}
		if selected == nil {
			selected = map[string]any{"id": e.ID, "name": e.ID, "reasoning": false, "input": []any{"text"}, "cost": map[string]any{"input": float64(0), "output": float64(0), "cacheRead": float64(0), "cacheWrite": float64(0)}}
			list = append(list, selected)
		}
		selected["contextWindow"], selected["maxTokens"] = float64(e.ContextWindow), float64(e.MaxTokens)
		p["models"] = list
	} else {
		cfg := get(filepath.Base(t.Path))
		cfg["model"] = provider + "/" + e.ID
		cfg["enabled_providers"] = []any{provider}
		providers, err := object(cfg, "provider")
		if err != nil {
			return nil, err
		}
		p, err := object(providers, provider)
		if err != nil {
			return nil, err
		}
		mergeOwned(p, map[string]any{"npm": "@ai-sdk/openai-compatible", "options": map[string]any{"baseURL": e.BaseURL, "apiKey": "local-qwen-tunnel"}, "models": map[string]any{e.ID: map[string]any{"limit": map[string]any{"context": float64(e.ContextWindow), "output": float64(e.MaxTokens)}}}})
		if p["name"] == nil {
			p["name"] = "Sovereign Kit"
		}
	}
	return out, nil
}

// mergeOwned changes only the explicitly supplied fields, preserving unrelated
// options, headers, compatibility settings and model-specific metadata.
func mergeOwned(destination, updates map[string]any) {
	for key, value := range updates {
		if patch, ok := value.(map[string]any); ok {
			child, ok := destination[key].(map[string]any)
			if !ok {
				child = map[string]any{}
				destination[key] = child
			}
			mergeOwned(child, patch)
		} else {
			destination[key] = value
		}
	}
}
func checkPackages(path string) error {
	for _, p := range []struct{ name, version string }{{"pi-subagents", "0.62.0"}, {"oh-my-pi", "0.2.0"}} {
		dir := filepath.Join(path, "npm/node_modules", p.name)
		v, err := readObject(filepath.Join(dir, "package.json"))
		if err != nil || v["name"] != p.name || v["version"] != p.version {
			return fmt.Errorf("required package %s@%s is missing or different", p.name, p.version)
		}
		if p.name == "oh-my-pi" {
			if info, err := os.Stat(filepath.Join(dir, "dist/extension.js")); err != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("Oh My Pi extension is incomplete")
			}
		}
		if p.name == "pi-subagents" {
			manifest, _ := v["pi"].(map[string]any)
			entries, _ := manifest["extensions"].([]any)
			if len(entries) == 0 {
				return fmt.Errorf("pi-subagents extension manifest is missing")
			}
			for _, value := range entries {
				entry, ok := value.(string)
				if !ok || entry == "" || filepath.IsAbs(entry) {
					return fmt.Errorf("unsafe pi-subagents entrypoint")
				}
				path := filepath.Join(dir, entry)
				rel, _ := filepath.Rel(dir, path)
				if rel == ".." || strings.HasPrefix(rel, "../") {
					return fmt.Errorf("unsafe pi-subagents entrypoint")
				}
				if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
					return fmt.Errorf("pi-subagents extension is incomplete")
				}
			}
		}
	}
	return nil
}

// Never follow symlinks in a confirmed destination or its ancestors.
func safePath(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("absolute profile path required")
	}
	for current := filepath.Clean(path); current != "/"; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe symlink: %s", current)
		}
	}
	return nil
}
func snapshot(ctx context.Context, t Target) (string, error) {
	hash := sha256.New()
	for _, name := range managed(t) {
		path := filepath.Join(root(t), name)
		if err := safePath(path); err != nil {
			return "", err
		}
		err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
			if os.IsNotExist(err) && p == path {
				io.WriteString(hash, name+":absent")
				return nil
			}
			if err != nil {
				return err
			}
			if err = ctx.Err(); err != nil {
				return err
			}
			rel, _ := filepath.Rel(root(t), p)
			fmt.Fprintf(hash, "%s:%s:%d:", rel, info.Mode(), info.Size())
			if info.Mode()&os.ModeSymlink != 0 {
				link, err := os.Readlink(p)
				if err != nil {
					return err
				}
				resolved, err := filepath.EvalSymlinks(p)
				if err != nil {
					return err
				}
				relative, err := filepath.Rel(root(t), resolved)
				if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
					return fmt.Errorf("symlink escapes profile: %s", p)
				}
				io.WriteString(hash, link)
				return nil
			}
			if info.IsDir() {
				return nil
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("unsafe special file: %s", p)
			}
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			_, err = io.Copy(hash, f)
			f.Close()
			return err
		})
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
