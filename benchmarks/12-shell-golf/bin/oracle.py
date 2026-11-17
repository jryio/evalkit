#!/usr/bin/env python3
# A stand-in agent that writes the reference script.
import json, os, sys
p = json.load(sys.stdin)
script = json.load(open(os.path.join(p["suite_dir"], "solutions.json")))[p["case"]["id"]]
open("solve.sh", "w").write(script + "\n")
