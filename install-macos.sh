#!/usr/bin/env bash
# Installs an isolated Pi + pi-subagents + Oh-My-Pi profile for private Qwen/SGLang.
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
profile_dir="${PI_SOVEREIGN_DIR:-$HOME/.pi/profiles/sovereign/agent}"
bin_dir="$HOME/.local/bin"

if ! command -v pi >/dev/null 2>&1; then
  cat >&2 <<'EOF'
Pi CLI is required first. Install Pi, reopen the terminal, then rerun this script.
See: https://github.com/badlogic/pi-mono
EOF
  exit 1
fi

mkdir -p "$profile_dir" "$bin_dir"
install -m 600 "$repo_dir/profile/settings.json" "$profile_dir/settings.json"
install -m 600 "$repo_dir/profile/models.json" "$profile_dir/models.json"
install -m 700 "$repo_dir/bin/pi-sovereign" "$bin_dir/pi-sovereign"
install -m 700 "$repo_dir/bin/qwen-sovereign-tunnel" "$bin_dir/qwen-sovereign-tunnel"

PI_CODING_AGENT_DIR="$profile_dir" pi install npm:pi-subagents@0.62.0
PI_CODING_AGENT_DIR="$profile_dir" pi install npm:oh-my-pi@0.2.0

cat <<EOF
Installed sovereign Pi profile in: $profile_dir

Ensure ~/.local/bin is on PATH, then use:
  qwen-sovereign-tunnel <ssh-host> <ssh-port>
  pi-sovereign
EOF
