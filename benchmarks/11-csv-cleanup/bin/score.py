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

import csv
sys.path.insert(0, os.path.join(suite, "bin"))
import ref
source = open(os.path.join(suite, "data", "people.csv")).read()
want = ref.run(case["id"], source)
path = os.path.join(cwd, "out.csv")
if open(os.path.join(cwd, "people.csv")).read() != source:
    emit("fail", "The agent changed people.csv, which it was told to leave alone.", values={"rows_out": 0, "rows_wrong": len(want)})
    sys.exit(0)
if not os.path.exists(path):
    emit("fail", "The agent wrote no out.csv.", values={"rows_out": 0, "rows_wrong": len(want)})
    sys.exit(0)
got = list(csv.reader(open(path).read().splitlines()))
wrong = sum(1 for i in range(max(len(got), len(want))) if (got[i] if i < len(got) else None) != (want[i] if i < len(want) else None))
table = lambda rows, label: {"kind": "table", "label": label, "width": "half", "columns": [{"key": "c%d" % i, "label": h} for i, h in enumerate(want[0])],
                             "rows": [{"c%d" % i: v for i, v in enumerate(r)} for r in rows[1:]]}
show = [table(got, "out.csv"), table(want, "The reference")]
values = {"rows_out": max(len(got) - 1, 0), "rows_wrong": wrong}
if wrong == 0:
    emit("pass", "out.csv matches the reference.", values=values, show=show)
else:
    emit("fail", "%d row(s) of out.csv differ from the reference." % wrong, values=values, show=show)
