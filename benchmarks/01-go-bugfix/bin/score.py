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

pkg = case["pkg"]
# The hidden test enters the workspace only now, after the agent is done.
src = os.path.join(suite, "hidden", pkg + "_test.go.txt")
dst = os.path.join(cwd, pkg, pkg + "_hidden_test.go")
open(dst, "w").write(open(src).read())
start = time.time()
r = subprocess.run(["go", "test", "./" + pkg], cwd=cwd, capture_output=True, text=True, timeout=120)
took = round(time.time() - start, 2)
os.remove(dst)
out = (r.stdout + r.stderr).strip()
tests = [{"name": "go test ./" + pkg, "status": "pass" if r.returncode == 0 else "fail", "duration": took, "output": out[-3000:]}]
emit("pass" if r.returncode == 0 else "fail",
     "The hidden tests pass." if r.returncode == 0 else "The hidden tests fail: " + (out.splitlines()[0] if out else "no output"),
     values={"test_s": took},
     show=[{"kind": "tests", "label": "Hidden tests", "tests": tests}])
