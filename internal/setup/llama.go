package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
)

// Precompiled images must already contain the runtime prerequisites. Legacy
// CUDA development images install Python/curl before the reconciliation lock.
func (launcher StrictSSHLauncher) prepareLlama(ctx context.Context, ssh config.SSH, r recipe.Recipe) error {
	if r.Runtime.Precompiled {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if launcher.OnLogs != nil {
			launcher.OnLogs(ServerLogs{Text: "Checking precompiled llama.cpp runtime…", CheckedAt: time.Now()})
		}
		command := `command -v python3 >/dev/null && command -v curl >/dev/null && test -x /opt/sovereign-kit/llama/llama-server && mkdir -p /workspace`
		if err := launcher.Runner.Run(ctx, strictSSHCommand(ssh, command)); err != nil {
			return fmt.Errorf("precompiled llama.cpp image is missing runtime prerequisites: %w", err)
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if launcher.OnLogs != nil {
		launcher.OnLogs(ServerLogs{Text: "Preparing CUDA image: installing Python and curl…", CheckedAt: time.Now()})
	}
	command := `mkdir -p /workspace && flock -w 120 /workspace/.sovkit-prerequisites.lock sh -c 'command -v python3 >/dev/null && command -v curl >/dev/null || { export DEBIAN_FRONTEND=noninteractive; apt-get -o Acquire::Retries=2 -o Acquire::http::Timeout=30 update && apt-get -o DPkg::Lock::Timeout=120 install -y python3 curl ca-certificates; }'`
	if err := launcher.Runner.Run(ctx, strictSSHCommand(ssh, command)); err != nil {
		return fmt.Errorf("prepare llama.cpp prerequisites: %w", err)
	}
	return nil
}

func ControlledLlamaCommand(r recipe.Recipe) (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	if r.Runtime.Engine != "llama-cpp" || r.Speculative == nil {
		return "", fmt.Errorf("llama.cpp launcher requires the reviewed native MTP recipe")
	}
	cacheK, cacheV := r.LlamaCacheTypes()
	// All dynamic values are JSON data, then shell-quoted as one Python argument.
	values, _ := json.Marshal(map[string]any{
		"source": r.Runtime.SourceRevision, "precompiled": r.Runtime.Precompiled, "repo": r.Model.Repository, "revision": r.Model.Revision,
		"file": r.Model.Filename, "sha256": r.Model.SHA256, "context": r.Serve.ContextWindow, "parallel": r.Serve.MaxRunningRequests,
		"cache_type_k": cacheK, "cache_type_v": cacheV,
	})
	script := "import json\nv=json.loads(" + fmt.Sprintf("%q", string(values)) + ")\n" + llamaBootstrap
	return controlledBackgroundCommand([]string{"python3", "-u", "-c", script, "--sovkit-llama-bootstrap", r.Model.Repository, r.Model.Revision}, "/workspace/sovkit-llama-cpp.log"), nil
}

// Python remains the supervised PID until exec. Its argv carries the recipe
// identity so an interrupted client can safely recognize a build/download.
const llamaBootstrap = `
import hashlib, os, pathlib, subprocess, time, urllib.request
root = pathlib.Path('/workspace')
def run(args):
    print('> ' + ' '.join(args), flush=True)
    subprocess.run(args, check=True, timeout=1200)
def sha(path):
    value = hashlib.sha256()
    with open(path, 'rb') as stream:
        for block in iter(lambda: stream.read(4 * 1024 * 1024), b''):
            value.update(block)
    return value.hexdigest()
if v.get('precompiled', False):
    print('Using precompiled native MTP runtime (no build required)', flush=True)
    prebuilt = pathlib.Path('/opt/sovereign-kit/llama')
    if (prebuilt/'source-revision').read_text().strip() != v['source']:
        raise RuntimeError('Precompiled llama.cpp source revision mismatch')
    binary = prebuilt/'llama-server'
else:
    # Compatibility only: old saved recipes used the CUDA development image.
    print('Preparing legacy source-build runtime (CUDA 13.3 / Blackwell 120a)', flush=True)
    os.environ['DEBIAN_FRONTEND'] = 'noninteractive'
    run(['apt-get', '-o', 'Acquire::Retries=2', '-o', 'Acquire::http::Timeout=30', 'update'])
    run(['apt-get', '-o', 'DPkg::Lock::Timeout=120', 'install', '-y', 'git', 'cmake', 'build-essential', 'libssl-dev', 'libcurl4-openssl-dev'])
    run(['nvidia-smi'])
    run(['/usr/local/cuda/bin/nvcc', '--version'])
    source = root / ('llama-' + v['source'])
    if not source.exists():
        run(['git', 'init', str(source)])
        run(['git', '-C', str(source), 'remote', 'add', 'origin', 'https://github.com/ggml-org/llama.cpp.git'])
    run(['git', '-C', str(source), 'fetch', '--depth', '1', 'origin', v['source']])
    run(['git', '-C', str(source), 'checkout', '--detach', v['source']])
    actual = subprocess.check_output(['git', '-C', str(source), 'rev-parse', 'HEAD'], text=True).strip()
    if actual != v['source']:
        raise RuntimeError('llama.cpp source revision mismatch')
    run(['cmake', '-S', str(source), '-B', str(source/'build'), '-DGGML_CUDA=ON', '-DCMAKE_BUILD_TYPE=Release', '-DCMAKE_CUDA_ARCHITECTURES=120a'])
    run(['cmake', '--build', str(source/'build'), '-j', str(min(os.cpu_count() or 2, 8)), '--target', 'llama-server'])
    binary = source/'build/bin/llama-server'
help_text = subprocess.check_output([str(binary), '--help'], text=True, stderr=subprocess.STDOUT)
for flag in ('draft-mtp', '--spec-draft-n-max', '--spec-draft-p-min'):
    if flag not in help_text:
        raise RuntimeError('Runtime lacks native MTP option: ' + flag)
model_dir = root/'models'/v['revision']
model_dir.mkdir(parents=True, exist_ok=True)
model = model_dir/v['file']
if not model.exists():
    partial = model.with_suffix('.gguf.partial')
    url = 'https://huggingface.co/' + v['repo'] + '/resolve/' + v['revision'] + '/' + v['file']
    print('Downloading pinned GGUF', flush=True)
    with urllib.request.urlopen(url, timeout=60) as response, open(partial, 'wb') as output:
        try:
            expected = int(response.headers.get('Content-Length', '0'))
        except (TypeError, ValueError):
            expected = 0
        if expected < 0 or expected > 9223372036854775807:
            expected = 0
        current = 0
        progress = 0
        last_report = time.monotonic()
        def report_download():
            print('SOVKIT_DOWNLOAD ' + json.dumps({'current': current, 'total': expected}), flush=True)
        report_download()
        while True:
            block = response.read(4 * 1024 * 1024)
            if not block:
                break
            output.write(block)
            current += len(block)
            if expected and current > expected:
                expected = 0
            now = time.monotonic()
            if current - progress >= 128 * 1024 * 1024 or now - last_report >= 1:
                # Reserve 100% for EOF; more bytes may still follow.
                if current != expected:
                    report_download()
                progress = current
                last_report = now
        output.flush()
        os.fsync(output.fileno())
        if current != expected:
            expected = 0
        report_download()
    print('Verifying GGUF SHA256', flush=True)
    if sha(partial) != v['sha256']:
        raise RuntimeError('GGUF SHA256 mismatch; refusing to load model')
    os.replace(partial, model)
elif sha(model) != v['sha256']:
    raise RuntimeError('Cached GGUF SHA256 mismatch; refusing to load model')
slots = v.get('parallel', 1)
args = [str(binary), '--model', str(model), '--alias', v['repo'],
    '--host', '127.0.0.1', '--port', '30000', '--ctx-size', str(v['context'] * slots),
    '--parallel', str(slots), '--kv-unified-per-slot', str(v['context']), '--metrics',
    '--gpu-layers', 'all', '--flash-attn', 'on',
    '--cache-type-k', v['cache_type_k'], '--cache-type-v', v['cache_type_v'], '--batch-size', '2048',
    '--ubatch-size', '512', '--spec-type', 'draft-mtp', '--spec-draft-n-max', '2',
    '--spec-draft-p-min', '0', '--jinja', '--no-warmup']
print('Launching llama-server: verified GGUF, native MTP depth 2, %d slots x %d context tokens' % (slots, v['context']), flush=True)
os.execv(args[0], args)
`
