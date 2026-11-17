#!/usr/bin/env python3
# A stand-in agent that applies the reference clean-up.
import json, os, subprocess, sys
p = json.load(sys.stdin)
ref = os.path.join(p["suite_dir"], "bin", "ref.py")
out = subprocess.run([sys.executable, ref, p["case"]["id"]], stdin=open("people.csv"), capture_output=True, text=True, check=True).stdout
open("out.csv", "w").write(out)
