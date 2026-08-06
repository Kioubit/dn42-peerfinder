#!/bin/sh
set -e

cd node-directory/

# Fetch latest changes from remote
git fetch origin

# Stage all local changes
git add .

# Commit local changes (don't fail if nothing to commit)
git commit -m "Auto-sync: $(date '+%Y-%m-%d %H:%M:%S')" || true

# Pull with rebase to maintain local priority
git pull --rebase origin main

# Push synchronized changes to remote
git push origin main

echo "Git sync completed successfully"

