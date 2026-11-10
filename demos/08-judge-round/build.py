#!/usr/bin/env python3
"""Write runs/round-3.jsonl: one judged support-reply draft per line.

Usage: python3 build.py            write every row
       python3 build.py --trickle  start empty and append one row a second, as a live loop does

This stands in for a hill-climbing harness: it generates a draft per case and rep,
asks an LLM judge for a verdict, three pass/fail criteria and a 0-10 quality score,
measures what it can without a judge, and appends the row. Judge answers go under
`proposed` so the reviewer can override them; measurements go under `values`, with
the baseline round's numbers for the same case. Rows the loop wants a human on get
a `flag`. The numbers are synthetic (seeded) and Acme is fictional.
"""
import json
import random
import sys
import time
from pathlib import Path

HERE = Path(__file__).resolve().parent
OUT = HERE / "runs" / "round-3.jsonl"

# (id, split, title, customer message, facts the reply must carry, a good reply)
CASES = [
    ("refund-annual", "test", "Refund for an annual plan bought by mistake",
     "I meant to buy the monthly plan and got charged $96 for a year. Can I get that back?",
     ["refund within 30 days", "switch to monthly", "3-5 business days"],
     "Sorry about that! Annual plans are refundable within 30 days, so you're covered. I've refunded the $96 and moved you to monthly; the refund shows up in 3-5 business days."),
    ("sync-stuck", "train", "Desktop sync stuck at 99%",
     "Acme Drive has said 'syncing 1 file' for two days. Restarting does nothing.",
     ["pause and resume", "file over 50 GB", "sync log path"],
     "That's usually one file over the 50 GB limit. Pause and resume sync from the menu bar, then check ~/Library/Logs/AcmeDrive/sync.log for the file name. If it's a big one, zip it or move it out of the folder."),
    ("share-external", "train", "Share a folder with an outside contractor",
     "How do I share one folder with a contractor without giving them access to everything?",
     ["share the folder only", "viewer or editor", "link expiry"],
     "Right-click the folder, choose Share, and add their email. Pick Viewer or Editor; they only see that folder. You can also set the link to expire, say after 30 days."),
    ("2fa-lost-phone", "test", "Locked out after losing a phone with 2FA",
     "My phone was stolen and I can't log in. 2FA codes went to that phone.",
     ["backup codes", "identity check", "24 hours"],
     "Sorry to hear that. If you saved backup codes, any one of them gets you in. If not, reply with a photo ID and we'll verify you; recovery takes up to 24 hours."),
    ("storage-full", "train", "Storage full warning on a 2 TB plan",
     "It says I'm out of space but I only have about 1.2 TB of files.",
     ["trash counts toward quota", "version history", "empty trash"],
     "Files in Trash and old versions still count toward your 2 TB. Empty Trash and trim version history under Settings > Storage, and you should see the space come back within an hour."),
    ("invoice-vat", "test", "Add a VAT number to past invoices",
     "Our finance team needs our VAT number on the last three invoices.",
     ["billing settings", "future invoices", "regenerate past invoices"],
     "Add the VAT number under Billing > Company details; it appears on every new invoice. For the last three, reply with the invoice numbers and we'll regenerate them with the VAT number."),
    ("linux-client", "train", "Is there a Linux desktop client?",
     "Do you have a Linux app? I'm on Ubuntu 24.04.",
     ["beta client", "deb package", "no selective sync yet"],
     "Yes, in beta: there's a .deb for Ubuntu 22.04 and later on the downloads page. One gap for now: selective sync isn't there yet, so it syncs the whole Drive."),
    ("deleted-file", "test", "Recover a file deleted last month",
     "I deleted a spreadsheet around five weeks ago and need it back.",
     ["30-day trash", "admin restore up to 90 days", "business plan"],
     "Trash keeps files for 30 days, so it's past that. On Business plans an admin can still restore files up to 90 days old from the Admin console; ask your admin to look under Deleted files."),
    ("seat-downgrade", "train", "Remove two seats mid-cycle",
     "We had two people leave. Can I drop to 8 seats now?",
     ["remove in admin console", "prorated credit", "next invoice"],
     "Sure. Remove the two users in the Admin console, then lower the seat count to 8. You'll get a prorated credit on your next invoice."),
    ("slow-upload", "train", "Uploads are slow from the office",
     "Uploads crawl at the office but are fine at home.",
     ["bandwidth limit setting", "proxy", "test upload speed"],
     "Check Preferences > Network: an upload limit may be set on the office machines. If that's off, your office proxy may be throttling; our speed test at status.acme.example tells you which."),
    ("sso-setup", "test", "Set up SSO with an identity provider",
     "We use Okta. How do we turn on SSO for our team?",
     ["SAML", "Business plan", "test with one user first"],
     "SSO uses SAML and is on Business plans. In the Admin console go to Security > SSO, paste your Okta metadata URL, and test with one user before you enforce it for everyone."),
    ("api-rate-limit", "train", "API returning 429 errors",
     "Our backup script started getting 429s from the API today.",
     ["600 requests per minute", "Retry-After header", "backoff"],
     "The API allows 600 requests a minute per token. A 429 includes a Retry-After header; wait that long and back off between retries, and the script will stay under the limit."),
    ("cancel-plan", "test", "Cancel and keep files",
     "If I cancel, do I lose my files right away?",
     ["read-only for 60 days", "download anytime", "deleted after 60 days"],
     "No. After you cancel, your Drive goes read-only for 60 days and you can download everything. After 60 days the files are deleted."),
    ("photo-backup", "train", "Phone photo backup stopped",
     "My iPhone stopped backing up photos last week.",
     ["background refresh", "low power mode", "open the app"],
     "iOS pauses backups in Low Power Mode or when Background App Refresh is off for Acme Drive. Turn those back on and open the app once to restart the backup."),
    ("student-discount", "train", "Student discount",
     "Do you have a student discount?",
     ["50% off", "school email", "renews yearly"],
     "We do: 50% off Personal with a school email address. Verification renews once a year."),
    ("outage-status", "test", "Is Acme Drive down?",
     "Nothing is loading for me in Europe. Is there an outage?",
     ["status page", "EU incident", "no data loss"],
     "Yes, there's an incident affecting EU users right now; the status page has live updates. No data is lost, and files will sync once it's resolved."),
]

# Ways a draft goes wrong, and what each does to the judge's answers.
FLAWS = {
    None: {},
    "invented": {"grounded": "no", "verdict": "fail"},
    "missed": {"answers": "no", "verdict": "fail"},
    "stiff": {"tone": "no"},
    "padded": {"tone": "no"},
}
PADDING = " I hope this helps! Please don't hesitate to reach out if you have any other questions or concerns, and thank you for choosing Acme Drive."
STIFF = "Dear Customer, thank you for contacting Acme Support. "


def draft(case, flaw, rng):
    _, _, _, _, facts, reply = case
    if flaw == "invented":
        return reply + f" As a courtesy we've also added {rng.choice(['$25', '$50', 'a free month'])} to your account.", facts
    if flaw == "missed":
        return reply.split(". ")[0] + ".", facts[:1]
    if flaw == "stiff":
        return STIFF + reply, facts
    if flaw == "padded":
        return reply + PADDING, facts
    return reply, facts


def row(case, rep, rng):
    cid, split, title, ask, facts, _ = case
    flaw = rng.choices(list(FLAWS), weights=[50, 14, 14, 11, 11])[0]
    text, covered = draft(case, flaw, rng)
    judged = {"verdict": "pass", "answers": "yes", "grounded": "yes", "tone": "yes", **FLAWS[flaw]}
    quality = max(0, min(10, round(rng.gauss(8.2 if flaw is None else 4.8, 1.2))))
    confidence = round(rng.uniform(0.55, 0.97) if flaw else rng.uniform(0.8, 0.99), 2)
    base_quality = max(0, min(10, quality + rng.choice([-3, -2, -1, -1, 0, 0, 1])))
    words = len(text.split())
    coverage = round(len(covered) / len(facts), 2)
    reasons = {
        None: "Answers the question with every required fact, in a plain, friendly voice.",
        "invented": "Offers a credit nobody approved; that is an invented commitment.",
        "missed": f"Stops after the first step and leaves out: {', '.join(facts[len(covered):])}.",
        "stiff": "Facts are right, but the 'Dear Customer' opener reads like a form letter.",
        "padded": "Facts are right, but the closing paragraph is filler.",
    }
    flag = None
    if confidence < 0.7:
        flag = f"Judge confidence {confidence:.2f}"
    elif split == "test" and judged["verdict"] == "fail":
        flag = "Held-out failure"
    item = {
        "id": f"{cid}.r{rep}",
        "group": split,
        "title": title,
        "short": title,
        "subtitle": f"rep {rep} · {words} words",
        "meta": f"claude-opus-5-5 · judge confidence {confidence:.2f}",
        "show": [
            {"id": "ask", "kind": "markdown", "label": "Customer", "text": ask},
            {"id": "draft", "kind": "markdown", "label": "Draft reply", "text": text},
        ],
        "context": [
            {"kind": "list", "label": "Facts the reply must carry",
             "rows": [{"id": f"f{i}", "title": f, "badge": {"label": "✓" if f in covered else "missing", "tone": "positive" if f in covered else "negative"}}
                      for i, f in enumerate(facts)]},
        ],
        "proposed": {
            "verdict": {"value": judged["verdict"], "why": f"Judge, confidence {confidence:.2f}: {reasons[flaw]}",
                        "baseline": "pass" if base_quality >= 6 else "fail"},
            "answers": {"value": judged["answers"]},
            "grounded": {"value": judged["grounded"]},
            "tone": {"value": judged["tone"]},
            "quality": {"value": quality, "why": reasons[flaw], "baseline": base_quality},
        },
        "values": {
            "coverage": {"value": coverage, "baseline": round(max(0, min(1, coverage - rng.choice([0, 0, 0.33, -0.33]))), 2)},
            "policy_ok": flaw != "invented",
            "words": {"value": words, "baseline": words + rng.randint(-10, 40)},
            "cost_usd": {"value": round(rng.uniform(0.008, 0.02), 4), "baseline": round(rng.uniform(0.008, 0.024), 4)},
            "latency_s": {"value": round(rng.uniform(3.5, 11), 1), "baseline": round(rng.uniform(4, 14), 1)},
        },
    }
    if flag:
        item["flag"] = flag
    return item


def main():
    rng = random.Random(3)
    rows = [row(case, rep, rng) for case in CASES for rep in range(2)]
    OUT.parent.mkdir(exist_ok=True)
    trickle = "--trickle" in sys.argv
    with OUT.open("w") as f:
        for r in rows:
            f.write(json.dumps(r) + "\n")
            if trickle:
                f.flush()
                time.sleep(1)
    print(f"wrote {len(rows)} rows to {OUT.relative_to(HERE)}")


if __name__ == "__main__":
    main()
