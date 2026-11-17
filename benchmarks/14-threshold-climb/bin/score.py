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

answer = json.loads(p["agent"]["output"])
want = case["expect"]["label"]
values = {"spam_score": answer["score"], "threshold": answer["threshold"]}
show = [{"kind": "text", "label": "Message", "text": case["text"]}]
if answer["label"] == want:
    emit("pass", "Labelled %s, which is right." % want, values=values, show=show)
else:
    emit("fail", "Labelled %s with a score of %d against a threshold of %d; it is %s." % (answer["label"], answer["score"], answer["threshold"], want), values=values, show=show)
