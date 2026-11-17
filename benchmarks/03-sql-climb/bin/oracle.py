#!/usr/bin/env python3
# A stand-in agent that answers every case with the reference query: evalkit check
# runs it to prove the scorer passes a right answer.
import json, os, sys
case = json.load(sys.stdin)["case"]
print(json.load(open(os.path.join(os.environ["EVALKIT_SUITE_DIR"], "solutions.json")))[case["id"]])
