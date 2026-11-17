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

import re, sqlite3

def query(sql):
    db = sqlite3.connect("file:" + os.path.join(cwd, "shop.db") + "?mode=ro", uri=True)
    try:
        return [tuple(round(v, 2) if isinstance(v, float) else v for v in row) for row in db.execute(sql).fetchall()]
    finally:
        db.close()

answer = p["agent"]["output"].strip()
fenced = re.search(r"```(?:sql)?\s*(.*?)```", answer, re.S)
sql = (fenced.group(1) if fenced else answer).strip().rstrip(";")
expect = case["expect"]
want = query(expect["sql"])
show = [{"kind": "code", "label": "The agent's query", "language": "sql", "code": sql or "(no query)"}]
try:
    got = query(sql)
except Exception as e:
    emit("fail", "The query does not run: %s." % e, values={"rows": 0}, show=show)
    sys.exit(0)
same = got == want if expect.get("ordered") else sorted(map(repr, got)) == sorted(map(repr, want))
table = lambda rows: {"kind": "table", "columns": [{"key": "c%d" % i, "label": "col %d" % (i + 1)} for i in range(max([len(r) for r in rows] or [1]))],
                      "rows": [{"c%d" % i: v for i, v in enumerate(r)} for r in rows[:20]]}
show += [dict(table(got), label="What it returned", width="half"), dict(table(want), label="The right result", width="half")]
if same:
    emit("pass", "The query returns the right result.", values={"rows": len(got)}, show=show)
else:
    emit("fail", "The query returns %s, and the right result is %s." % (str(got[:3])[:120], str(want[:3])[:120]), values={"rows": len(got)}, show=show)
