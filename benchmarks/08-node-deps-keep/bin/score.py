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

name = case["id"]
os.makedirs(os.path.join(cwd, "test"), exist_ok=True)
dst = os.path.join(cwd, "test", name + ".test.js")
open(dst, "w").write(open(os.path.join(suite, "hidden", name + ".test.js")).read())
start = time.time()
r = subprocess.run(["node", "--test", dst], cwd=cwd, capture_output=True, text=True, timeout=120)
took = round(time.time() - start, 2)
os.remove(dst)
log = os.path.join(suite, ".work", "installs.log")
installs = len(open(log).readlines()) if os.path.exists(log) else 0
out = (r.stdout + r.stderr).strip()
tests = [{"name": "node --test " + name, "status": "pass" if r.returncode == 0 else "fail", "duration": took, "output": out[-2500:]}]
reason = next((l.strip() for l in out.splitlines() if "AssertionError" in l or "Error:" in l), "the test failed")
emit("pass" if r.returncode == 0 else "fail",
     "The hidden test passes." if r.returncode == 0 else "The hidden test fails: " + reason[:200],
     values={"test_s": took, "installs": installs},
     show=[{"kind": "tests", "label": "Hidden test", "tests": tests}])
