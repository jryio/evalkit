#!/bin/sh
# A scripted agent: what it does is the case's "behaviour" field.
behaviour=$(sed -n 's/.*"behaviour": *"\([a-z-]*\)".*/\1/p' | head -1)
tries="$EVALKIT_SUITE_DIR/.work/$EVALKIT_RUN-$EVALKIT_CASE.tries"
mkdir -p "$EVALKIT_SUITE_DIR/.work"
echo try >> "$tries"
case "$behaviour" in
  slow) sleep 1 ;;
  crash) echo "segmentation fault (simulated)" >&2; exit 139 ;;
  hang) sleep 60 ;;
  hang-once) [ "$(wc -l < "$tries")" -gt 1 ] || sleep 60 ;;
esac
echo "done after $(wc -l < "$tries" | tr -d ' ') try"
