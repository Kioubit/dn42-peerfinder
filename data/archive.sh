#!/bin/sh
set -eu

exec 9>access.lock
flock 9

rm -f archive.zip

# Use a subshell so the directory change is scoped
(
  cd node-directory
  zip -r -X -9 -q ../archive.zip . \
    -i "servers/*.yml" "LICENSE" "peerfinder.py"
)

zip -X -9 -q archive.zip README.txt

