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

want = case["expect"]["answer"]
got = p["agent"]["output"].strip().strip("`").strip()
head = subprocess.run(["git", "log", "-1", "--format=%s"], cwd=cwd, capture_output=True, text=True).stdout.strip()
if got == want:
    emit("pass", "The agent reported what this commit prints.")
else:
    emit("fail", "The agent answered %r and this commit (%s) gives %r." % (got[:80], head, want))
