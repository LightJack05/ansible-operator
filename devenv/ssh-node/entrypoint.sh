#!/bin/bash
set -e

if [ -f /tmp/authorized_keys.pub ]; then
    install -o ansible -g ansible -m 600 /tmp/authorized_keys.pub /home/ansible/.ssh/authorized_keys
    install -o root    -g root    -m 600 /tmp/authorized_keys.pub /root/.ssh/authorized_keys
fi

exec /usr/sbin/sshd -D -e
