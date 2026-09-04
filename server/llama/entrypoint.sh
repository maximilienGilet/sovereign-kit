#!/bin/sh
set -eu
# Never distribute SSH host identities. Generate missing keys per container,
# preserving its identity across restarts and Vast's supplied startup command.
ssh-keygen -A
exec /opt/nvidia/nvidia_entrypoint.sh "$@"
