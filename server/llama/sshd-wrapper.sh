#!/bin/sh
set -eu
# Vast replaces the image entrypoint. Initialize identity at the daemon boundary
# instead, including when Vast launches sshd through the distribution service.
# ssh-keygen -A preserves existing keys when a container restarts.
/usr/bin/ssh-keygen -A
mkdir -p /run/sshd
exec /usr/sbin/sshd.sovkit-original "$@"
