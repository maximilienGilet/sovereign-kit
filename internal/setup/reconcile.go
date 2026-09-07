package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
)

// Reconcile runs both inspection and dispatch under a remote OS lock. An intent
// without a provably matching process is deliberately not retried automatically.
func (launcher StrictSSHLauncher) Reconcile(ctx context.Context, ssh config.SSH, r recipe.Recipe) error {
	return launcher.reconcile(ctx, ssh, r, false)
}

// Restart is only for an explicitly approved, confirmed stopped instance.
// The fingerprint, process inventory and port checks still apply.
func (launcher StrictSSHLauncher) Restart(ctx context.Context, ssh config.SSH, r recipe.Recipe) error {
	return launcher.reconcile(ctx, ssh, r, true)
}

func (launcher StrictSSHLauncher) reconcile(ctx context.Context, ssh config.SSH, r recipe.Recipe, restart bool) error {
	if launcher.Runner == nil {
		return fmt.Errorf("server command runner is required")
	}
	if err := validateSSH(ssh); err != nil {
		return err
	}
	if err := r.Validate(); err != nil {
		return err
	}
	var command string
	var err error
	switch r.Runtime.Engine {
	case "sglang":
		command, err = ControlledSGLangCommand(r)
	case "vllm":
		command, err = ControlledVLLMCommand(r)
	case "llama-cpp":
		command, err = ControlledLlamaCommand(r)
	default:
		return fmt.Errorf("unsupported reconciliation engine")
	}
	if err != nil {
		return err
	}
	if r.Runtime.Engine == "llama-cpp" {
		if err := launcher.prepareLlama(ctx, ssh, r); err != nil {
			return err
		}
	}
	script := reconciliationScript(r, command, "/workspace/.sovkit-deployment", "/proc")
	if restart {
		script = restartReconciliationScript(r, command, "/workspace/.sovkit-deployment", "/proc")
	}
	if err := launcher.Runner.Run(ctx, strictSSHCommand(ssh, "python3 -c "+shellQuote(script))); err != nil {
		// A refused resume must still expose the previous launch's diagnostics.
		// This is read-only: never remove the marker or redispatch the server.
		launcher.captureServerLogs(ctx, ssh, r.Runtime.Engine)
		return fmt.Errorf("reconcile remote deployment: %w", err)
	}
	// Readiness is always rechecked, including a reused in-progress deployment.
	if err := launcher.waitServerReadyWithAdvice(ctx, ssh, r.Runtime.Engine, llamaOOMAdvice(r)); err != nil {
		return fmt.Errorf("deployment is not healthy yet; resume to observe it: %w", err)
	}
	return nil
}

func reconciliationScript(r recipe.Recipe, command, marker, proc string) string {
	snapshot, _ := json.Marshal(r)
	values, _ := json.Marshal(map[string]string{"fingerprint": digest(snapshot), "model": r.Model.Repository, "revision": r.Model.Revision, "file": r.Model.Filename, "engine": r.Runtime.Engine, "command": command, "marker": marker, "proc": proc})
	return "import json, os, fcntl, subprocess, sys\nv = json.loads(" + fmt.Sprintf("%q", string(values)) + ")\n" + `
def refuse(message):
    print(message, file=sys.stderr)
    sys.exit(1)
# Minimal container images need not provide /workspace. Initialize it before
# opening the lock; exist_ok preserves existing deployments and concurrent starts.
os.makedirs(os.path.dirname(v['marker']), mode=0o700, exist_ok=True)
with open(v['marker'] + '.lock', 'a') as lock:
    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    processes = []
    for pid in os.listdir(v['proc']):
        if not pid.isdigit() or int(pid) == os.getpid():
            continue
        try:
            args = open(os.path.join(v['proc'], pid, 'cmdline'), 'rb').read().decode().split('\x00')
        except FileNotFoundError:
            continue
        # Any server engine or process claiming the serving port is relevant.
        if any(os.path.basename(a) in ('vllm', 'sglang') or a.startswith(('sglang.', 'vllm.')) for a in args) or (args and os.path.basename(args[0]) == 'llama-server') or '30000' in args or '--sovkit-llama-bootstrap' in args:
            processes.append(args)
    previous = None
    try:
        with open(v['marker']) as source:
            previous = json.load(source)
    except FileNotFoundError:
        pass
    if processes:
        if previous != v['fingerprint']:
            refuse('Unrecognized server or deployment fingerprint; refusing duplicate launch')
        # Never assume extra engine processes are related worker processes.
        # Operators must resolve ambiguous inventories before continuing.
        def matches(a):
            if v['engine'] == 'llama-cpp':
                path = '/workspace/models/' + v['revision'] + '/' + v['file']
                return v['model'] in a and (path in a or ('--sovkit-llama-bootstrap' in a and v['revision'] in a))
            return v['model'] in a and v['revision'] in a
        parents = [a for a in processes if matches(a)]
        if len(parents) != 1 or len(processes) != 1:
            refuse('Cannot prove one matching pinned server; inspect remote processes')
        sys.exit(0)
    # One narrowly proven pre-exec failure from the old launcher is recoverable.
    # No server was executed; do not generalize this to model/runtime crashes.
    recover_missing_command = False
    old_log = os.path.join(os.path.dirname(v['marker']), 'sovkit-sglang.log')
    if previous == v['fingerprint'] and v['engine'] == 'sglang':
        try:
            with open(old_log, 'rb') as source:
                failure = source.read(4096)
            recover_missing_command = failure.strip() == b"nohup: failed to run command 'sglang': No such file or directory"
        except FileNotFoundError:
            pass
    if previous is not None and not recover_missing_command:
        refuse('Previous launch outcome is uncertain; inspect remote deployment before retrying')
    # Check the serving port even when a process does not advertise it in argv.
    import socket
    probe = socket.socket()
    try:
        probe.bind(('127.0.0.1', 30000))
    except OSError:
        refuse('Serving port is occupied; refusing duplicate launch')
    finally:
        probe.close()
    if recover_missing_command:
        # Preserve evidence, and consume the exception before dispatch. A crash
        # after this point cannot use the stale log to authorize another launch.
        os.link(old_log, old_log + '.missing-command')
        os.unlink(old_log)
    else:
        with open(v['marker'], 'x') as output:
            os.chmod(v['marker'], 0o600)
            json.dump(v['fingerprint'], output)
            output.flush()
            os.fsync(output.fileno())
    directory = os.open(os.path.dirname(v['marker']), os.O_RDONLY)
    os.fsync(directory)
    os.close(directory)
    subprocess.run(v['command'], shell=True, check=True)
`
}

func restartReconciliationScript(r recipe.Recipe, command, marker, proc string) string {
	script := reconciliationScript(r, command, marker, proc)
	script = strings.Replace(script, "if previous is not None and not recover_missing_command:", "if previous != v['fingerprint']:", 1)
	script = strings.Replace(script, "    else:\n        with open(v['marker'], 'x') as output:", "    elif previous is None:\n        with open(v['marker'], 'x') as output:", 1)
	return script
}
