#!/usr/bin/env bash
# Vendors the parts of @pierre/diffs evalkit uses (scripts/vendor-diffs/entry.js) as one
# self-contained ES module at web/vendor/diffs.esm.js. build.mjs stubs out what evalkit
# never runs, so only the diff view and evalkit's languages ship.
# Usage: scripts/vendor-diffs.sh 1.6.4
set -euo pipefail

version="${1:?usage: scripts/vendor-diffs.sh VERSION}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out="$root/web/vendor/diffs.esm.js"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

npm install --prefix "$tmp" --ignore-scripts --no-audit --no-fund --silent "@pierre/diffs@$version" esbuild@0.27
cp "$root"/scripts/vendor-diffs/entry.js "$root"/scripts/vendor-diffs/build.mjs "$tmp/"
(cd "$tmp" && node build.mjs diffs.mjs licenses.txt)
cp "$tmp/licenses.txt" "$root/web/vendor/diffs.LICENSES.txt"

{
  echo "/* @pierre/diffs $version (Apache-2.0) with Shiki (MIT), bundled by esbuild. Regenerate: scripts/vendor-diffs.sh $version */"
  cat "$tmp/diffs.mjs"
} > "$tmp/out.mjs"

cp "$tmp/out.mjs" "$out"
echo "wrote $out: $(wc -c < "$out" | tr -d ' ') bytes, $(gzip -c "$out" | wc -c | tr -d ' ') gzipped"
