#!/bin/sh
# Passes every attempt that reached it, and counts the tries it took. For
# the breaks-scorer case it prints something that is not JSON, as a broken
# scorer would.
if [ "$EVALKIT_CASE" = breaks-scorer ]; then
  echo "Traceback (most recent call last): KeyError"
  exit 0
fi
tries=$(wc -l < "$EVALKIT_SUITE_DIR/.work/$EVALKIT_RUN-$EVALKIT_CASE.tries" | tr -d ' ')
printf '{"proposed":{"verdict":{"value":"pass","why":"The agent finished."}},"values":{"tries":%s}}' "$tries"
