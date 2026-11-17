#!/usr/bin/env python3
# A stand-in agent that makes the rename exactly: the JSON key in the API and
# the property the client reads, and nothing that merely contains the old name.
import json, sys
case = json.load(sys.stdin)["case"]
old, new = case["old"], case["new"]
for path, before, after in (("api/server.py", '"%s"' % old, '"%s"' % new), ("web/client.js", "u." + old, "u." + new)):
    text = open(path).read()
    open(path, "w").write(text.replace(before, after))
