#!/bin/sh
# Stands in for "npm install": puts the dependency in the slot and records
# that an install happened, so a row can show how many there were.
set -eu
mkdir -p app/node_modules
cp -R "$EVALKIT_SUITE_DIR/vendor/padleft" app/node_modules/padleft
echo "$(date -u +%FT%TZ) $EVALKIT_SLOT" >> "$EVALKIT_SUITE_DIR/.work/installs.log"
