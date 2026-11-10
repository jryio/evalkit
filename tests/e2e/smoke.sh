#!/usr/bin/env bash
# Opens every item of a running evalkit server and fails on render errors.
# Usage: tests/e2e/smoke.sh <port>
set -euo pipefail
PORT=${1:?usage: smoke.sh <port>}
BASE="http://127.0.0.1:$PORT"
S="agent-browser --session smoke-$PORT"
IDS=$(curl -fsS "$BASE/api/suite" | python3 -c 'import json,sys; d=json.load(sys.stdin); print("\n".join(i["id"] for i in (d.get("suite") or {}).get("items", [])))')
[ -n "$IDS" ] || { echo "no items (fatal suite?)"; exit 1; }
fail=0
$S open "$BASE/" >/dev/null
for id in $IDS; do
  $S open "$BASE/#$id" >/dev/null
  $S wait 400 >/dev/null
  bad=$($S eval 'document.querySelectorAll(".artifact-error").length + (document.querySelector("h1") ? 0 : 100)' | tr -d '"')
  if [ "$bad" != "0" ]; then
    echo "FAIL $id: $($S eval '[...document.querySelectorAll(".artifact-error")].map(e => e.textContent).join(" | ") || document.body.innerText.slice(0, 200)')"
    fail=1
  else
    echo "ok   $id"
  fi
done
errs=$($S errors || true)
[ -z "$errs" ] || { echo "JS errors:"; echo "$errs"; fail=1; }
$S close >/dev/null
exit $fail
