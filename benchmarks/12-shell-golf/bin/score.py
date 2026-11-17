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

path = os.path.join(cwd, "solve.sh")
inputs = case["expect"]["inputs"]
if not os.path.exists(path):
    emit("fail", "The agent wrote no solve.sh.", values={"chars": 0, "inputs_passed": 0})
    sys.exit(0)
script = open(path).read()
tests, passed = [], 0
for i, t in enumerate(inputs):
    # The script is the agent's: it runs in the attempt's own directory with a short limit.
    try:
        r = subprocess.run(["sh", "solve.sh"], cwd=cwd, input=t["in"], capture_output=True, text=True, timeout=10)
        ok, out = r.stdout == t["out"], "printed %r, want %r" % (r.stdout, t["out"])
    except subprocess.TimeoutExpired:
        ok, out = False, "did not finish in 10 seconds"
    passed += ok
    tests.append({"name": "hidden input %d" % (i + 1), "status": "pass" if ok else "fail", "output": out})
values = {"chars": len(script.strip()), "inputs_passed": passed}
show = [{"kind": "code", "label": "solve.sh", "language": "bash", "code": script}, {"kind": "tests", "label": "Hidden inputs", "tests": tests}]
if passed == len(inputs):
    emit("pass", "The script is right on every hidden input.", values=values, show=show)
else:
    emit("fail", "The script is wrong on %d of %d hidden inputs." % (len(inputs) - passed, len(inputs)), values=values, show=show)
