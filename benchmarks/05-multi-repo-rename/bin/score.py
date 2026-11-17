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

old, new = case["old"], case["new"]
api, web = p["sources"]["api"], p["sources"]["web"]
want = "name: Ada\nsince: 2026-01-05\nspent: 1200\nadmin: yes\naddress: 1 Main St\n"
out = subprocess.run("python3 server.py | node ../web/client.js", shell=True, cwd=api, capture_output=True, text=True, timeout=60)
record = subprocess.run(["python3", "server.py"], cwd=api, capture_output=True, text=True, timeout=60).stdout
stale = [f for f in ("api/server.py", "web/client.js") if old in open(os.path.join(p["workspace"], f)).read().replace(new, "")]
changed = sum(len([l for l in d.splitlines() if l.startswith("+++ ")]) for d in p.get("diff", {}).values())
values = {"files_changed": changed}
if out.returncode != 0 or out.stdout != want:
    emit("fail", "The client no longer renders the API's record: " + (out.stderr.strip().splitlines() or ["the output differs"])[-1][:160], values=values)
elif '"%s"' % new not in record:
    emit("fail", "The API's JSON has no field named %s." % new, values=values)
elif stale:
    emit("fail", "The old name %s is still in %s." % (old, " and ".join(stale)), values=values)
else:
    emit("pass", "The API returns %s and the client reads it." % new, values=values)
