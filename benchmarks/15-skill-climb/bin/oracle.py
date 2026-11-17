#!/usr/bin/env python3
# A stand-in agent that adds a line in the house format.
import json, sys
p = json.load(sys.stdin)
kinds = {"retry": "fixed", "json-flag": "added", "drop-py38": "removed", "faster-search": "changed", "crash-empty": "fixed"}
text = open("CHANGELOG.md").read()
line = "- [%s] %s" % (kinds[p["case"]["id"]], p["case"]["change"].split(";")[0].split(",")[0].rstrip(".") + ".")
open("CHANGELOG.md", "w").write(text.replace("## Unreleased\n\n", "## Unreleased\n\n" + line[:90].rstrip(".") + ".\n", 1))
