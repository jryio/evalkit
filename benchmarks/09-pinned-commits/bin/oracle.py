#!/usr/bin/env python3
# A stand-in agent that runs the command the prompt names and prints its output.
import json, subprocess, sys
case = json.load(sys.stdin)["case"]
cmd = ["git", "rev-list", "--all", "--count"] if case.get("kind") == "count" else ["python3", "app.py"]
print(subprocess.run(cmd, capture_output=True, text=True).stdout.strip())
