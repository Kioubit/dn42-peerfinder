#!/bin/sh
set -eu

exec 9>access.lock
flock 9

cd node-directory/

# Abort any previous failed rebase
[ -d ".git/rebase-merge" ] && git rebase --abort

# Stage all changes
git add -A

# Commit if there are local changes
if ! git diff --cached --quiet; then
    git commit -m "Auto-sync: $(date '+%Y-%m-%d %H:%M:%S')"
fi

# Fetch and rebase (forcing local changes to win conflicts)
git fetch origin main
git rebase -X theirs origin/main

# Push (retry once if push fails due to network/race)
git push origin main || {
    echo "First push failed. Retrying after fetch..."
    git fetch origin main
    git rebase -X theirs origin/main
    git push origin main
}

