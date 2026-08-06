#!/bin/sh
set -e

rm archive.zip || true

cd node-directory
zip -r -X -9 -q ../archive.zip . -x "./*.txt" "./*.md" "*/.*" "./.*" "./*.sh"
cd ..
zip -X -9 -q archive.zip README.txt

