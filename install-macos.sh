#!/usr/bin/env bash
# Install the CLI. Model-specific profiles are opt-in from a connected dashboard.
set -euo pipefail
umask 077

install_opencode=0
for arg in "$@"; do
  case "$arg" in
    --upgrade) ;; # Retained for compatibility; never replaces client profiles.
    --with-opencode) install_opencode=1 ;;
    *) printf 'Usage: %s [--upgrade] [--with-opencode]\n' "${0##*/}" >&2; exit 64 ;;
  esac
done

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bin_dir="$HOME/.local/bin"
if ! command -v go >/dev/null 2>&1; then
  printf 'Go is required to build the sovkit CLI.\n' >&2
  exit 1
fi
if [[ "$install_opencode" -eq 1 ]]; then
  if ! command -v npm >/dev/null 2>&1; then
    printf 'npm is required for the explicitly requested OpenCode installation.\n' >&2
    exit 1
  fi
  npm install --global opencode-ai@1.18.25
fi

stage_dir="$(mktemp -d "${TMPDIR:-/tmp}/sovkit-install.XXXXXX")"
cleanup() { rm -f "$stage_dir/sovkit"; rmdir "$stage_dir"; }
trap cleanup EXIT
(cd "$repo_dir" && go build -o "$stage_dir/sovkit" ./cmd/sovkit)
install -d -m 700 "$bin_dir"
install -m 700 "$stage_dir/sovkit" "$bin_dir/sovkit"
install -m 700 "$repo_dir/bin/sovkit" "$bin_dir/sovkit-doctor"
install -m 700 "$repo_dir/bin/sovkit-tunnel" "$bin_dir/sovkit-tunnel"
install -m 700 "$repo_dir/bin/pi-sovereign" "$bin_dir/pi-sovereign"
install -m 700 "$repo_dir/bin/opencode-sovereign" "$bin_dir/opencode-sovereign"

printf 'Installed Sovereign Kit CLI in: %s\n' "$bin_dir"
printf 'Existing Pi and OpenCode profiles were not changed.\n'
printf '\nEnsure ~/.local/bin is on PATH, then run sovkit up to provision a private route.\n'
