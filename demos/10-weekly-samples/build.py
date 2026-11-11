#!/usr/bin/env python3
"""Write runs/<week>.jsonl: one production support reply per line, sampled fresh each week.

Usage: python3 build.py

This stands in for production monitoring: every Monday a job samples 20 tickets the
Acme support assistant answered that week, asks an LLM judge for a verdict, and
records each reply's latency and cost. Every week has new tickets, so no row pairs
with another week's row; the weeks compare as wholes. Judge answers go under
`proposed`, measurements under `values`, and every reply the judge failed gets a
`flag`. In the week of 2026-10-26 an EU incident brought a wave of outage tickets,
and the assistant answered most of them from a stale status page. The numbers are
synthetic (seeded) and Acme is fictional.
"""
import datetime
import json
import random
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.dont_write_bytecode = True
sys.path.insert(0, str(HERE.parent / "08-judge-round"))
from build import CASES, draft  # noqa: E402

FIRST_WEEK = datetime.date(2026, 10, 5)
PASSES = [16, 17, 16, 12, 15, 16]  # of 20, per week
INCIDENT_WEEK = 3
OUTAGE_TICKETS = 6
STALE = "We're not seeing any incident on our side, and the status page shows all systems operational. Try signing out and back in."
OPENERS = ["", "Hi, ", "Hello, ", "Hey team, ", "Quick question: "]
CLOSERS = ["", " Thanks!", " Thanks in advance.", " This is blocking my team."]
PLANS = ["Personal", "Team", "Business"]


def reason(flaw, facts, covered):
    return {
        None: "Answers the question with every required fact, in a plain, friendly voice.",
        "invented": "Offers a credit nobody approved; that is an invented commitment.",
        "missed": f"Stops after the first step and leaves out: {', '.join(facts[len(covered):])}.",
        "stale": "Says there is no incident while the EU incident was open; the reply trusted a stale status page.",
    }[flaw]


def row(ticket, case, flaw, week, rng):
    _, _, title, ask, facts, _ = case
    text, covered = (STALE, []) if flaw == "stale" else draft(case, flaw, rng)
    confidence = round(rng.uniform(0.6, 0.97) if flaw else rng.uniform(0.8, 0.99), 2)
    why = reason(flaw, facts, covered)
    item = {
        "id": f"t-{ticket}",
        "title": title,
        "short": title,
        "subtitle": f"#{ticket} · {len(text.split())} words",
        "meta": f"{rng.choice(PLANS)} plan · judge confidence {confidence:.2f}",
        "show": [
            {"id": "ask", "kind": "markdown", "label": "Customer", "text": rng.choice(OPENERS) + ask + rng.choice(CLOSERS)},
            {"id": "reply", "kind": "markdown", "label": "Assistant reply", "text": text},
        ],
        "proposed": {
            "verdict": {"value": "fail" if flaw else "pass", "why": f"Judge, confidence {confidence:.2f}: {why}"},
        },
        "values": {
            "latency_s": round(max(1.5, rng.gauss(9.0 if week == INCIDENT_WEEK else 6.5, 1.5)), 1),
            "cost_usd": round(max(0.004, rng.gauss(0.011, 0.002)), 4),
        },
    }
    if flaw:
        item["flag"] = "Judge failed this reply"
    return item


def week_rows(week, n_pass, rng):
    outage = next(c for c in CASES if c[0] == "outage-status")
    others = [c for c in CASES if c is not outage]
    n_outage = OUTAGE_TICKETS if week == INCIDENT_WEEK else 0
    cases = [outage] * n_outage + [rng.choice(others) for _ in range(20 - n_outage)]
    # Ticket numbers grow week over week, so no id repeats across runs.
    tickets = sorted(rng.sample(range(48200 + 1400 * week, 48200 + 1400 * (week + 1)), 20))
    n_stale = max(0, n_outage - 1)  # one outage ticket still got the right answer
    flaws = ["stale"] * n_stale + [None] * (20 - n_stale)
    for i in rng.sample(range(n_outage, 20), 20 - n_pass - n_stale):
        flaws[i] = rng.choice(["invented", "missed"])
    order = list(range(20))
    rng.shuffle(order)
    return [row(tickets[k], cases[i], flaws[i], week, rng) for k, i in enumerate(order)]


def main():
    rng = random.Random(10)
    (HERE / "runs").mkdir(exist_ok=True)
    for week, n_pass in enumerate(PASSES):
        run = (FIRST_WEEK + datetime.timedelta(weeks=week)).isoformat()
        rows = week_rows(week, n_pass, rng)
        with (HERE / "runs" / f"{run}.jsonl").open("w") as f:
            for r in rows:
                f.write(json.dumps(r) + "\n")
        passed = sum(r["proposed"]["verdict"]["value"] == "pass" for r in rows)
        print(f"{run}  pass {passed / len(rows):.2f}  flagged {sum('flag' in r for r in rows)}")


if __name__ == "__main__":
    main()
