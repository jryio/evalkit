#!/usr/bin/env bash
# Vendors Observable Plot, bundled with its d3 modules, as one self-contained
# ES module at web/vendor/plot.esm.js. Usage: scripts/vendor-plot.sh 0.6.19
set -euo pipefail

version="${1:?usage: scripts/vendor-plot.sh VERSION}"
url="https://esm.sh/@observablehq/plot@$version/es2022/plot.bundle.mjs"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out="$root/web/vendor/plot.esm.js"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

curl -fsSL "$url" -o "$tmp/download.mjs"

if ! head -n 1 "$tmp/download.mjs" | grep -qF "@observablehq/plot@$version "; then
  echo "unexpected banner: $(head -c 120 "$tmp/download.mjs")" >&2
  exit 1
fi

{
  echo "/* Observable Plot $version, bundled with its d3 modules by esm.sh. ISC license. Regenerate: scripts/vendor-plot.sh $version */"
  # Drop esm.sh's banner and the source map link; the map is not vendored.
  grep -vE '^(/\* esm\.sh - |//# sourceMappingURL=)' "$tmp/download.mjs"
} > "$tmp/plot.mjs"

if grep -nE '(^|[^.])import[ (]|from ?"' "$tmp/plot.mjs"; then
  echo "bundle imports other modules; it must be self-contained" >&2
  exit 1
fi

types="$(node -e 'import(require("node:url").pathToFileURL(process.argv[1])).then(m => console.log(typeof m.plot, typeof m.dot, typeof m.line, typeof m.ruleY))' "$tmp/plot.mjs")"
if [[ "$types" != "function function function function" ]]; then
  echo "bundle exports are wrong: $types" >&2
  exit 1
fi

cp "$tmp/plot.mjs" "$out"
echo "wrote $out: $(wc -c < "$out" | tr -d ' ') bytes, $(gzip -c "$out" | wc -c | tr -d ' ') gzipped"
