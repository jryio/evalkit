#!/usr/bin/env python3
# Measures the message's format. It sets no verdict: accuracy is the judge's call.
import json, re, sys
p = json.load(sys.stdin)
message = p["agent"]["output"].strip()
message = re.sub(r"^```\w*\n|\n```$", "", message).strip()
lines = message.split("\n")
subject = lines[0] if lines else ""
errors = []
if len(subject) > 72:
    errors.append("the subject is %d characters, over 72" % len(subject))
if subject.endswith("."):
    errors.append("the subject ends with a period")
if len(lines) > 1 and lines[1].strip():
    errors.append("there is no blank line after the subject")
if re.match(r"(?i)(added|fixed|changed|updated|removed)\b", subject):
    errors.append("the subject is in the past tense")
row = {"values": {"subject_len": len(subject), "format_errors": len(errors)},
       "show": [{"kind": "text", "label": "Commit message", "text": message or "(empty)"},
                {"kind": "diff", "label": "The diff it describes", "diff": p["case"]["diff"]}]}
if errors:
    row["flag"] = "Format: " + "; ".join(errors) + "."
print(json.dumps(row))
