#!/bin/bash
set -e

# Install the authorized key from the mount (if present).
if [ -f /tmp/authorized_keys.pub ]; then
    install -o git -g git -m 600 /tmp/authorized_keys.pub /home/git/.ssh/authorized_keys
fi

# Make sure mounted bare repos are owned by git and HTTP-push enabled.
chown -R git:git /srv/git || true
find /srv/git -maxdepth 2 -name '*.git' -type d 2>/dev/null | while read -r repo; do
    git -C "$repo" config http.receivepack true || true
done

# fcgiwrap (CGI bridge for git-http-backend) + nginx.
service fcgiwrap start
nginx

# sshd in the foreground so the container stays alive.
exec /usr/sbin/sshd -D -e
