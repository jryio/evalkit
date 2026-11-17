#!/usr/bin/env python3
# The "agent" of this benchmark: runs the implementation named by $IMPL on the
# case's file and reports its answer and how long it took.
import json, os, subprocess, sys, time
p = json.load(sys.stdin)
impl = os.path.join(p["suite_dir"], "impls", os.environ.get("IMPL", "naive") + ".py")
start = time.perf_counter()
r = subprocess.run([sys.executable, impl, p["case"]["file"]], capture_output=True, text=True)
ms = (time.perf_counter() - start) * 1000
if r.returncode != 0:
    sys.stderr.write(r.stderr)
    sys.exit(r.returncode)
print(json.dumps({"output": json.dumps({"top": json.loads(r.stdout), "ms": round(ms, 1)})}))
