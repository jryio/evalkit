#!/usr/bin/env python3
# A scripted proposer: it moves the threshold toward the kind of mistake the
# best version makes most, by a step that halves after every reverted round.
import json
failures = json.load(open("failures.json"))
missed_spam = sum(1 for f in failures if f["case"]["expect"]["label"] == "spam")
flagged_ham = len(failures) - missed_spam
reverted = open("history.md").read().count("no (reverted)")
step = max(1, 3 >> reverted)
path = "targets/config/filter.json"
config = json.load(open(path))
before = config["threshold"]
if not failures:
    print("Nothing fails on the training messages, so the threshold stays at %d." % before)
else:
    config["threshold"] = before - step if missed_spam >= flagged_ham else before + step
    open(path, "w").write(json.dumps(config, indent=2) + "\n")
    print("Moved the threshold from %d to %d: the best version misses %d spam and flags %d real messages." % (before, config["threshold"], missed_spam, flagged_ham))
