#!/bin/sh
set -eu
dpkg-divert --local --add --rename --divert /usr/sbin/sshd.sovkit-original /usr/sbin/sshd
install -m 755 /opt/sovereign-kit/llama/sshd-wrapper.sh /usr/sbin/sshd
# The service must match the actual daemon executable when stopping/reloading,
# but start through the key-initializing wrapper. Keep package-owned originals
# diverted so an apt install during Vast startup cannot overwrite the wrappers.
dpkg-divert --local --add --rename --divert /etc/init.d/ssh.sovkit-original /etc/init.d/ssh
sed \
    -e 's@--exec /usr/sbin/sshd@--exec /usr/sbin/sshd.sovkit-original --startas /usr/sbin/sshd@g' \
    -e 's@status_of_proc -p /run/sshd.pid /usr/sbin/sshd @status_of_proc -p /run/sshd.pid /usr/sbin/sshd.sovkit-original @g' \
    /etc/init.d/ssh.sovkit-original > /etc/init.d/ssh
chmod 755 /etc/init.d/ssh
