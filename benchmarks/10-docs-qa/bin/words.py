#!/usr/bin/env python3
# Measures the answer's length. The verdict is the judge's.
import json, sys
p = json.load(sys.stdin)
answer = p["agent"]["output"].strip()
print(json.dumps({"values": {"words": len(answer.split())},
                  "show": [{"kind": "text", "label": "Question", "text": p["case"]["prompt"]},
                           {"kind": "markdown", "label": "Answer", "text": answer or "(no answer)"},
                           {"kind": "text", "label": "Reference answer", "text": p["case"]["expect"]["answer"]}]}))
