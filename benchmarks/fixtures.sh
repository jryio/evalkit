#!/bin/sh
# Turns each benchmark's template directories into the git repositories its
# workspace clones, under <benchmark>/.work/ (not committed). Run it once
# before `evalkit run`, and again after editing a template.
#
# A line of <benchmark>/fixture.list is: <template dir> <repo name> [history script]
# The history script, if any, makes the commits itself; otherwise there is one.
set -eu
cd "$(dirname "$0")"
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null
export GIT_AUTHOR_NAME=fixture GIT_AUTHOR_EMAIL=fixture@example.com
export GIT_COMMITTER_NAME=fixture GIT_COMMITTER_EMAIL=fixture@example.com
export GIT_AUTHOR_DATE=2026-01-01T00:00:00Z GIT_COMMITTER_DATE=2026-01-01T00:00:00Z

for list in */fixture.list; do
  bench=$(dirname "$list")
  while read -r template name history; do
    [ -n "$template" ] || continue
    repo="$bench/.work/$name"
    rm -rf "$repo"
    mkdir -p "$repo"
    cp -R "$bench/$template/." "$repo/"
    git -C "$repo" init -q -b main
    if [ -n "${history:-}" ]; then
      (cd "$repo" && sh "../../$history")
    else
      git -C "$repo" add -A
      git -C "$repo" commit -q -m "fixture"
    fi
    echo "made $repo ($(git -C "$repo" rev-list --count HEAD) commit(s))"
  done < "$list"
done
