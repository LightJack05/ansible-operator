#!/bin/bash
set -e

# The bare repositories under /srv/git are baked into the image at build time
# (see create-repos.sh); this entrypoint only starts the services.

# Install an authorized key if one was provided (optional; password auth with
# git/gitpass works without it, so no mount is required).
if [ -f /tmp/authorized_keys.pub ]; then
    install -o git -g git -m 600 /tmp/authorized_keys.pub /home/git/.ssh/authorized_keys
fi

# Ensure SSH host keys exist (containers often ship without them).
ssh-keygen -A

# fcgiwrap runs as 'git' so git-http-backend can write for pushes and cgit can
# read the repos. Socket is group-owned by www-data so nginx can reach it.
spawn-fcgi -u git -g git -U git -G www-data -M 0660 -s /run/fcgiwrap.socket -- /usr/sbin/fcgiwrap

nginx

# sshd in the foreground so the container stays alive.
exec /usr/sbin/sshd -D -e
