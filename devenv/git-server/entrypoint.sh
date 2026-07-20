#!/bin/bash
set -e
shopt -s nullglob

# Turn every subdirectory of /data into a bare repo under /srv/git.
# Each /data/<name> becomes /srv/git/<name>.git with one "Initial Commit"
# snapshotting the directory contents.
mkdir -p /srv/git
for src in /data/*/; do
    name=$(basename "$src")
    repo="/srv/git/${name}.git"

    if [ ! -d "$repo" ]; then
        git init --bare -b main "$repo"
        git --git-dir="$repo" --work-tree="$src" add -A
        git --git-dir="$repo" --work-tree="$src" \
            -c user.name='Test'  -c user.email='test@example.com' \
            commit -m "Initial Commit" || true
    fi
done

# Install the authorized key from the mount (if present).
if [ -f /tmp/authorized_keys.pub ]; then
    install -o git -g git -m 600 /tmp/authorized_keys.pub /home/git/.ssh/authorized_keys
fi

# Enable HTTP push, a cgit description, and dumb-http info on every bare repo
# (covers both the ones created above and any pre-made ones mounted in).
find /srv/git -maxdepth 2 -name '*.git' -type d -print0 | while IFS= read -r -d '' repo; do
    git -C "$repo" config http.receivepack true || true
    [ -s "$repo/description" ] || basename "${repo%.git}" > "$repo/description"
    git -C "$repo" update-server-info || true
done
chown -R git:git /srv/git

# Ensure SSH host keys exist (containers often ship without them).
ssh-keygen -A

# fcgiwrap runs as 'git' so git-http-backend can write for pushes and cgit can
# read the repos. Socket is group-owned by www-data so nginx can reach it.
spawn-fcgi -u git -g git -U git -G www-data -M 0660 -s /run/fcgiwrap.socket -- /usr/sbin/fcgiwrap

nginx

# sshd in the foreground so the container stays alive.
exec /usr/sbin/sshd -D -e
