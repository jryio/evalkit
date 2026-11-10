#!/usr/bin/env bash
# Keys-only review flow against a running server: answer, edit, send, keep going,
# send again. Fails unless exactly two new snapshots arrive and the page stays up.
# Usage: tests/e2e/flow.sh <port>
set -euo pipefail
PORT=${1:?usage: flow.sh <port>}
BASE="http://127.0.0.1:$PORT"
S="agent-browser --session flow-$PORT"
seq() { curl -fsS "$BASE/api/version" | python3 -c 'import json,sys; print(json.load(sys.stdin)["sequence"])'; }
before=$(seq)
$S open "$BASE/" >/dev/null
$S wait 800 >/dev/null
$S press 1 >/dev/null
if [ "$($S eval '!!document.querySelector("[data-focus^=\"a:\"]")')" = "true" ]; then
  $S press e >/dev/null
  $S press End >/dev/null
  $S type '[data-focus^="a:"]' " (edited in e2e)" >/dev/null
  # Blur directly: agent-browser's Escape right after `type` stops page timers (seen on a bare page too).
  $S eval 'document.activeElement.blur(); 1' >/dev/null
fi
$S press j >/dev/null
$S press 2 >/dev/null
$S press Meta+Enter >/dev/null
$S wait 300 >/dev/null
$S press Enter >/dev/null
$S wait 600 >/dev/null
$S press j >/dev/null
$S press 1 >/dev/null
$S press Meta+Enter >/dev/null
$S wait 300 >/dev/null
$S press Enter >/dev/null
$S wait 600 >/dev/null
alive=$($S eval '!!document.querySelector("h1") && !document.querySelector(".artifact-error")')
errs=$($S errors || true)
$S close >/dev/null
after=$(seq)
if [ "$alive" != "true" ] || [ -n "$errs" ] || [ $((after - before)) -ne 2 ]; then
  echo "FAIL port $PORT: alive=$alive sends=$((after - before)) errors=$errs"
  exit 1
fi
echo "ok   port $PORT: 2 sends, page alive"
