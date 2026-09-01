#!/usr/bin/env bash
# Installs an isolated Pi + pi-subagents + Oh-My-Pi profile for private Qwen/SGLang.
# Existing profiles are preserved unless --upgrade is supplied.
set -euo pipefail
umask 077

upgrade=0
if [[ $# -eq 1 && "$1" == "--upgrade" ]]; then
  upgrade=1
elif [[ $# -ne 0 ]]; then
  printf 'Usage: %s [--upgrade]\n' "${0##*/}" >&2
  exit 64
fi

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
profile_dir="${PI_SOVEREIGN_DIR:-$HOME/.pi/profiles/sovereign/agent}"
profile_parent="$(dirname "$profile_dir")"
stage_dir="$profile_parent/.agent.staging.$$"
bin_dir="$HOME/.local/bin"
backup_dir=""

if ! command -v pi >/dev/null 2>&1; then
  cat >&2 <<'EOF'
Pi CLI is required first. Install Pi, reopen the terminal, then rerun this script.
See: https://github.com/badlogic/pi-mono
EOF
  exit 1
fi
if [[ -e "$profile_dir" && "$upgrade" -ne 1 ]]; then
  printf 'Refusing to overwrite existing profile: %s\nRe-run with --upgrade to back it up and replace it.\n' "$profile_dir" >&2
  exit 73
fi

cleanup() { [[ -d "$stage_dir" ]] && rm -rf "$stage_dir"; }
trap cleanup EXIT
mkdir -p "$stage_dir" "$bin_dir"
install -m 600 "$repo_dir/profile/settings.json" "$stage_dir/settings.json"
install -m 600 "$repo_dir/profile/models.json" "$stage_dir/models.json"

PI_CODING_AGENT_DIR="$stage_dir" pi install npm:pi-subagents@0.62.0
PI_CODING_AGENT_DIR="$stage_dir" pi install npm:oh-my-pi@0.2.0

python3 - "$stage_dir" <<'PY'
import json, pathlib, sys
profile = pathlib.Path(sys.argv[1])
settings = json.loads((profile / 'settings.json').read_text())
models = json.loads((profile / 'models.json').read_text())
assert settings['subagents']['modelScope'] == {
    'enforce': True, 'strict': True, 'allow': ['sovereign-qwen/*']
}
assert list(models['providers']) == ['sovereign-qwen']
assert (profile / 'npm/node_modules/pi-subagents').exists()
assert (profile / 'npm/node_modules/oh-my-pi/dist/extension.js').exists()
PY

if [[ -e "$profile_dir" ]]; then
  backup_dir="$profile_dir.backup.$(date +%Y%m%d%H%M%S)"
  mv "$profile_dir" "$backup_dir"
fi
mkdir -p "$profile_parent"
mv "$stage_dir" "$profile_dir"
install -m 700 "$repo_dir/bin/pi-sovereign" "$bin_dir/pi-sovereign"
install -m 700 "$repo_dir/bin/qwen-sovereign-tunnel" "$bin_dir/qwen-sovereign-tunnel"
trap - EXIT

printf 'Installed sovereign Pi profile in: %s\n' "$profile_dir"
[[ -n "$backup_dir" ]] && printf 'Previous profile backed up in: %s\n' "$backup_dir"
printf '\nEnsure ~/.local/bin is on PATH, then use:\n  qwen-sovereign-tunnel <ssh-host> <ssh-port> <ssh-user> <identity-file> <known-hosts-file>\n  pi-sovereign\n'
