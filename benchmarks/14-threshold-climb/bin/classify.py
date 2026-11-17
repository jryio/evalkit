#!/usr/bin/env python3
# The system under test: adds up the weights of the keywords in a message
# and calls it spam at or above the threshold. Its config is filter.json.
import json, re, sys
case = json.load(sys.stdin)["case"]
config = json.load(open("filter.json"))
words = set(re.findall(r"[a-z]+", case["text"].lower()))
score = sum(w for word, w in config["weights"].items() if word in words)
print(json.dumps({"output": json.dumps({"label": "spam" if score >= config["threshold"] else "ham", "score": score, "threshold": config["threshold"]})}))
