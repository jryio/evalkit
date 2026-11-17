#!/usr/bin/env python3
import json, os, subprocess, sys, time
p = json.load(sys.stdin)
case, cwd, suite = p["case"], p["cwd"], p["suite_dir"]

def emit(verdict, why, values=None, flag=None, show=None):
    row = {"proposed": {"verdict": {"value": verdict, "why": why}}}
    if values: row["values"] = values
    if flag: row["flag"] = flag
    if show: row["show"] = show
    print(json.dumps(row))

import collections
answer = json.loads(p["agent"]["output"])
counts = collections.Counter(open(os.path.join(cwd, case["file"])).read().split())
want = [[w, n] for w, n in sorted(counts.items(), key=lambda c: (-c[1], c[0]))[:10]]
values = {"ms": answer["ms"], "words": case["words"]}
show = [{"kind": "table", "label": "The ten most common words", "columns": [{"key": "word", "label": "Word"}, {"key": "n", "label": "Count", "align": "right"}],
         "rows": [{"word": w, "n": n} for w, n in answer["top"]]}]
if answer["top"] == want:
    emit("pass", "The ten words and their counts are right.", values=values, show=show)
else:
    emit("fail", "The answer differs from the reference count.", values=values, show=show, flag="Wrong answer")
