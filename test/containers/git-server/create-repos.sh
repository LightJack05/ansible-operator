#!/bin/bash
set -e
shopt -s nullglob

# Build-time variant of the devenv entrypoint's repo creation: turn every
# subdirectory of /data into a bare repo under /srv/git. Each /data/<name>
# becomes /srv/git/<name>.git with one "Initial Commit" snapshotting the
# directory contents.
mkdir -p /srv/git
for src in /data/*/; do
    name=$(basename "$src")
    repo="/srv/git/${name}.git"

    git init --bare -b main "$repo"
    git --git-dir="$repo" --work-tree="$src" add -A
    git --git-dir="$repo" --work-tree="$src" \
        -c user.name='Test'  -c user.email='test@example.com' \
        commit -m "Initial Commit"

    # Enable HTTP push, a cgit description, and dumb-http info.
    git -C "$repo" config http.receivepack true
    basename "${repo%.git}" > "$repo/description"
    git -C "$repo" update-server-info
done
chown -R git:git /srv/git
