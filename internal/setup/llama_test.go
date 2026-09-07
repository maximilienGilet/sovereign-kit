package setup

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
)

func llamaRecipe(t *testing.T) recipe.Recipe {
	t.Helper()
	r, err := recipe.Load("../../recipes/qwen-solo-uncensored.toml")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestOfficialSoloPassesPinnedArtifactToLlamaLauncher(t *testing.T) {
	r, err := recipe.Load("../../recipes/qwen-solo-rtx5090.toml")
	if err != nil {
		t.Fatal(err)
	}
	command, err := ControlledLlamaCommand(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"unsloth/Qwen3.8-27B-GGUF", "Qwen3.8-27B-UD-Q4_K_XL.gguf", "4ca720788d1e01f1bff70c033e0d0028fd02e502", "3f227079003add2511437e5b1e94812e363385225bf6a9b47b0054a72bc8b01e"} {
		if !strings.Contains(command, want) {
			t.Fatalf("launcher lost artifact pin %q", want)
		}
	}
	if strings.Contains(command, "HauhauCS") {
		t.Fatal("official Solo launches Uncensored artifact")
	}
}

func TestPrecompiledPreflightNeverFallsBackToInstallation(t *testing.T) {
	for _, present := range []bool{false, true} {
		dir := t.TempDir()
		binary := filepath.Join(dir, "llama-server")
		for _, tool := range []string{"apt-get", "cmake"} {
			if err := os.WriteFile(filepath.Join(dir, tool), []byte("#!/bin/sh\nexit 91\n"), 0700); err != nil {
				t.Fatal(err)
			}
		}
		if present {
			if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
				t.Fatal(err)
			}
		}
		r := llamaRecipe(t)
		r.Runtime.Precompiled = true
		runner := &fakeCommandRunner{onRun: func(c Command) error {
			remote := strings.ReplaceAll(c.Args[len(c.Args)-1], "/opt/sovereign-kit/llama/llama-server", binary)
			remote = strings.ReplaceAll(remote, "/workspace", filepath.Join(dir, "workspace"))
			command := exec.Command("sh", "-c", remote)
			command.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			output, err := command.CombinedOutput()
			if err != nil {
				t.Logf("preflight %s: %s", remote, output)
			}
			return err
		}}
		launcher := StrictSSHLauncher{Runner: runner}
		err := launcher.prepareLlama(context.Background(), config.SSH{}, r)
		if (err == nil) != present {
			t.Fatalf("binary present=%v, preflight error=%v", present, err)
		}
	}
}

func TestLlamaLaunchWaitsForReadinessAndStreamsLogs(t *testing.T) {
	var logs string
	runner := &fakeCommandRunner{onOutput: func(c Command) ([]byte, error) { return []byte("ready\nllama ready\n"), nil }}
	l := StrictSSHLauncher{Runner: runner, OnLogs: func(s ServerLogs) { logs = s.Text }}
	err := l.Launch(context.Background(), config.SSH{Host: "host", Port: 22, User: "root", IdentityFile: "/key", KnownHostsFile: "/hosts"}, llamaRecipe(t))
	if err != nil || logs != "llama ready" {
		t.Fatalf("launch/readiness: %v logs=%q", err, logs)
	}
}

func TestLlamaReconciliationRecognizesServerAndBootstrap(t *testing.T) {
	r := llamaRecipe(t)
	for _, argv := range [][]string{
		{"/workspace/llama/build/bin/llama-server", "--model", "/workspace/models/" + r.Model.Revision + "/" + r.Model.Filename, "--alias", r.Model.Repository, "--port", "30000"},
		{"python3", "-u", "-c", "bootstrap", "--sovkit-llama-bootstrap", r.Model.Repository, r.Model.Revision},
		{"llama-server", "--model", "/foreign/model.gguf"},
	} {
		dir := t.TempDir()
		proc := filepath.Join(dir, "proc")
		marker := filepath.Join(dir, "deployment")
		if err := os.MkdirAll(filepath.Join(proc, "123"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(proc, "123", "cmdline"), []byte(strings.Join(argv, "\x00")+"\x00"), 0600); err != nil {
			t.Fatal(err)
		}
		if slices.Contains(argv, "--sovkit-llama-bootstrap") {
			if err := os.MkdirAll(filepath.Join(proc, "124"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(proc, "124", "cmdline"), []byte("cmake\x00--build\x00/workspace/llama/build\x00--target\x00llama-server\x00"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		snapshot, _ := json.Marshal(r)
		fingerprint, _ := json.Marshal(digest(snapshot))
		if err := os.WriteFile(marker, fingerprint, 0600); err != nil {
			t.Fatal(err)
		}
		script := reconciliationScript(r, "exit 99", marker, proc)
		output, err := exec.Command("python3", "-c", script).CombinedOutput()
		foreign := argv[0] == "llama-server"
		if (err != nil) != foreign {
			t.Fatalf("argv=%v err=%v output=%s", argv, err, output)
		}
	}
}

// Execute the real supervisor with real files/hashing. Substitute only the
// network and GPU/compiler/process-exec boundaries, unavailable on this Mac.
func TestLlamaSupervisorVerifiesDownloadBeforeExec(t *testing.T) {
	for _, mode := range []string{"success", "bad-hash", "build-fails", "missing-mtp", "prebuilt", "prebuilt-parallel", "prebuilt-q4", "prebuilt-wrong-source", "builtin-official", "builtin-dual", "builtin-dual-max", "builtin-uncensored", "length-missing", "length-invalid", "length-negative", "length-short", "length-long", "progress-time", "progress-bounded"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			r := llamaRecipe(t)
			if mode == "prebuilt-q4" {
				r.Runtime.KVCacheTypeK = "q4_0"
				r.Runtime.KVCacheTypeV = "q4_0"
			}
			if mode == "builtin-official" {
				var err error
				r, err = recipe.Load("../../recipes/qwen-solo-rtx5090.toml")
				if err != nil {
					t.Fatal(err)
				}
			}
			if mode == "builtin-dual" {
				var err error
				r, err = recipe.Load("../../recipes/qwen-solo-dual.toml")
				if err != nil {
					t.Fatal(err)
				}
			}
			if mode == "builtin-dual-max" {
				var err error
				r, err = recipe.Load("../../recipes/qwen-solo-dual-max.toml")
				if err != nil {
					t.Fatal(err)
				}
			}
			precompiled := strings.HasPrefix(mode, "prebuilt")
			if strings.HasPrefix(mode, "builtin-") {
				precompiled = r.Runtime.Precompiled
			}
			ctx, slots := 262144, 1
			if mode == "prebuilt-parallel" {
				ctx, slots = 65536, 4
			}
			if strings.HasPrefix(mode, "builtin-") {
				ctx, slots = r.Serve.ContextWindow, r.Serve.MaxRunningRequests
			}
			cacheK, cacheV := r.LlamaCacheTypes()
			values, _ := json.Marshal(map[string]any{"source": r.Runtime.SourceRevision, "repo": r.Model.Repository, "revision": r.Model.Revision, "file": r.Model.Filename, "sha256": "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", "context": ctx, "parallel": slots, "precompiled": precompiled, "cache_type_k": cacheK, "cache_type_v": cacheV})
			prebuilt := filepath.Join(dir, "prebuilt")
			if err := os.Mkdir(prebuilt, 0700); err != nil {
				t.Fatal(err)
			}
			revision := r.Runtime.SourceRevision
			if mode == "prebuilt-wrong-source" {
				revision = "wrong"
			}
			if err := os.WriteFile(filepath.Join(prebuilt, "source-revision"), []byte(revision), 0600); err != nil {
				t.Fatal(err)
			}
			quotedDir, _ := json.Marshal(dir)
			quotedMode, _ := json.Marshal(mode)
			prefix := `import json, os, subprocess, urllib.request, io, time
root_path = ` + string(quotedDir) + `
mode = ` + string(quotedMode) + `
v = json.loads(` + strconv.Quote(string(values)) + `)
def fake_run(args, **kwargs):
    if mode.startswith(('prebuilt', 'builtin-')):
        raise RuntimeError('Prebuilt deployment attempted build/package command: ' + str(args))
    if args[:2] == ['cmake', '--build'] and mode == 'build-fails':
        raise subprocess.CalledProcessError(1, args)
    with open(root_path+'/commands', 'a') as f: f.write(json.dumps(args)+'\n')
subprocess.run = fake_run
subprocess.check_output = lambda args, **kwargs: (v['source'] if args[0] == 'git' else ('no MTP' if mode == 'missing-mtp' else 'draft-mtp --spec-draft-n-max --spec-draft-p-min'))
elapsed = 0
class SlowResponse(io.BytesIO):
    def read(self, size=-1):
        global elapsed
        elapsed += 0.6 if mode == 'progress-time' else 0.1
        return super().read(1)
if mode.startswith('progress-'):
    time.monotonic = lambda: elapsed
def download(url, **kwargs):
    assert url == 'https://huggingface.co/'+v['repo']+'/resolve/'+v['revision']+'/'+v['file']
    response = (SlowResponse if mode.startswith('progress-') else io.BytesIO)(b'corrupt' if mode == 'bad-hash' else b'abc')
    length = {'length-missing': None, 'length-invalid': 'oops', 'length-negative': '-1', 'length-short': '2', 'length-long': '4'}.get(mode, '3')
    response.headers = {} if length is None else {'Content-Length': length}
    return response
urllib.request.urlopen = download
def launch(binary, args):
    with open(root_path+'/launched.json', 'w') as f: json.dump(args, f)
    raise SystemExit(0)
os.execv = launch
`
			script := prefix + strings.Replace(strings.Replace(llamaBootstrap, "root = pathlib.Path('/workspace')", "root = pathlib.Path(root_path)", 1), "pathlib.Path('/opt/sovereign-kit/llama')", "pathlib.Path(root_path)/'prebuilt'", 1)
			output, err := exec.Command("python3", "-c", script).CombinedOutput()
			prebuiltSuccess := strings.HasPrefix(mode, "prebuilt") && mode != "prebuilt-wrong-source"
			if mode != "success" && !prebuiltSuccess && !strings.HasPrefix(mode, "builtin-") && !strings.HasPrefix(mode, "length-") && !strings.HasPrefix(mode, "progress-") {
				if err == nil {
					t.Fatalf("%s did not fail", mode)
				}
				if _, e := os.Stat(filepath.Join(dir, "launched.json")); !os.IsNotExist(e) {
					t.Fatal("failed preparation launched server")
				}
				return
			}
			if err != nil {
				t.Fatalf("supervisor: %v\n%s", err, output)
			}
			var counters []struct{ Current, Total int64 }
			for _, line := range strings.Split(string(output), "\n") {
				if payload, ok := strings.CutPrefix(line, "SOVKIT_DOWNLOAD "); ok {
					var counter struct{ Current, Total int64 }
					if err := json.Unmarshal([]byte(payload), &counter); err != nil {
						t.Fatal(err)
					}
					counters = append(counters, counter)
				}
			}
			wantReports := 2
			if mode == "progress-time" {
				wantReports = 3
			}
			if len(counters) != wantReports || counters[0].Current != 0 || counters[len(counters)-1].Current != 3 {
				t.Fatalf("expected measured start and EOF counters: %v\n%s", counters, output)
			}
			wantTotal := int64(3)
			if strings.HasPrefix(mode, "length-") {
				wantTotal = 0
			}
			if counters[len(counters)-1].Total != wantTotal {
				t.Fatalf("EOF total=%d want %d", counters[len(counters)-1].Total, wantTotal)
			}
			if mode == "progress-time" && counters[1].Current != 2 {
				t.Fatalf("intermediate progress not measured: %v", counters)
			}
			data, err := os.ReadFile(filepath.Join(dir, "launched.json"))
			if err != nil {
				t.Fatal(err)
			}
			var args []string
			if err := json.Unmarshal(data, &args); err != nil {
				t.Fatal(err)
			}
			if (mode == "prebuilt" || strings.HasPrefix(mode, "builtin-")) && args[0] != filepath.Join(prebuilt, "llama-server") {
				t.Fatalf("not using prebuilt binary: %v", args)
			}
			totalContext := strconv.Itoa(ctx * slots)
			if mode == "builtin-official" {
				if ctx != 262144 || slots != 1 || r.Serve.MaxOutputTokens != 16384 {
					t.Fatal("official recipe lost full-context single-slot limits")
				}
			}
			for flag, want := range map[string]string{"--host": "127.0.0.1", "--port": "30000", "--ctx-size": totalContext, "--parallel": strconv.Itoa(slots), "--kv-unified-per-slot": strconv.Itoa(ctx), "--gpu-layers": "all", "--cache-type-k": cacheK, "--cache-type-v": cacheV, "--batch-size": "2048", "--ubatch-size": "512", "--spec-type": "draft-mtp", "--spec-draft-n-max": "2", "--spec-draft-p-min": "0", "--alias": r.Model.Repository} {
				i := slices.Index(args, flag)
				if i < 0 || i+1 >= len(args) || args[i+1] != want {
					t.Errorf("%s: %v", flag, args)
				}
			}
			if !slices.Contains(args, "--metrics") {
				t.Fatal("missing server metrics")
			}
			model, err := os.ReadFile(filepath.Join(dir, "models", r.Model.Revision, r.Model.Filename))
			if err != nil || string(model) != "abc" {
				t.Fatal("verified download not committed")
			}
		})
	}
}
