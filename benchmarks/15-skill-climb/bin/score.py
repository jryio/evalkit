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

import re
before = open(os.path.join(suite, "template", "CHANGELOG.md")).read().splitlines()
path = os.path.join(cwd, "CHANGELOG.md")
after = open(path).read().splitlines() if os.path.exists(path) else []
added = [l for l in after if l.strip() and l not in before]
want = case["expect"]["kind"]
broken = []
if len(added) != 1:
    broken.append("it must add exactly one line, and it added %d" % len(added))
line = added[0] if added else ""
if added:
    section = after[after.index("## Unreleased") + 1:] if "## Unreleased" in after else []
    first = next((l for l in section if l.strip()), "")
    if first != line:
        broken.append("the new line must be the first one under the Unreleased heading")
    m = re.match(r"^- \[(added|changed|fixed|removed)\] (.*)$", line)
    if not m:
        broken.append("the line must start with a dash and a kind in square brackets: [added], [changed], [fixed] or [removed]")
    else:
        if m.group(1) != want:
            broken.append("the kind must be [%s], and it is [%s]" % (want, m.group(1)))
        if not m.group(2)[:1].isupper():
            broken.append("the sentence must start with a capital letter")
        if not m.group(2).endswith("."):
            broken.append("the sentence must end with a period")
    if len(line) > 90:
        broken.append("the line must be at most 90 characters, and it is %d" % len(line))
values = {"rules_broken": len(broken)}
show = [{"kind": "code", "label": "The line the agent added", "code": "\n".join(added) or "(nothing added)"}]
if broken:
    emit("fail", "The entry breaks the house format: " + "; ".join(broken) + ".", values=values, show=show)
else:
    emit("pass", "The entry follows the house format.", values=values, show=show)
