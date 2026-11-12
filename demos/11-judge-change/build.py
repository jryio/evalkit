#!/usr/bin/env python3
"""Write runs/rubric-v1.jsonl and runs/rubric-v2.jsonl: the same 32 drafts, graded under two judge rubrics.

Usage: python3 build.py

This stands in for a judge change: before a hill-climbing loop switches rubrics, it
regrades one round's support-reply drafts (the cases of demo 08, twice each) with the
old rubric and the new one. Both files hold the same drafts with the same ids, reps
and `show`; only the judge's answers under `proposed` differ. rubric-v2 checks
`grounded` against the help-center article, so a claim the article does not make now
fails, and a detail the article does make now passes. Rows whose verdict flips get a
`flag`. The numbers are synthetic (seeded) and Acme is fictional.
"""
import json
import random
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.dont_write_bytecode = True
sys.path.insert(0, str(HERE.parent / "08-judge-round"))
from build import CASES, draft  # noqa: E402

# How many of the 32 drafts have each flaw. "unsourced" passes v1 and fails v2;
# "quoted" fails v1 and passes v2.
FLAW_COUNTS = {None: 14, "stiff": 2, "padded": 2, "invented": 3, "missed": 3, "unsourced": 7, "quoted": 1}
UNSOURCED = [" This usually takes effect within the hour.", " It works the same way on every plan.",
             " Most customers see it fixed right away.", " You'll also get an email confirmation."]
QUOTED = " The help-center article on this has a screenshot of each step."
JUDGE_COST = {"v1": 0.004, "v2": 0.0065}  # $ per grade; v2 sends the help-center article


def write_draft(case, flaw, rng):
    if flaw == "unsourced":
        text, covered = draft(case, None, rng)
        return text + rng.choice(UNSOURCED), covered
    if flaw == "quoted":
        text, covered = draft(case, None, rng)
        return text + QUOTED, covered
    return draft(case, flaw, rng)


def grade(rubric, flaw, facts, covered):
    """The judge's answers and reason for one draft under one rubric."""
    strict = rubric == "v2"
    grounded = not (flaw == "invented" or (flaw == "unsourced" and strict) or (flaw == "quoted" and not strict))
    answers = flaw != "missed"
    tone = flaw not in ("stiff", "padded")
    reasons = {
        None: "Answers the question with every required fact, in a plain, friendly voice.",
        "stiff": "Facts are right, but the 'Dear Customer' opener reads like a form letter.",
        "padded": "Facts are right, but the closing paragraph is filler.",
        "invented": "Offers a credit nobody approved; that is an invented commitment.",
        "missed": f"Stops after the first step and leaves out: {', '.join(facts[len(covered):])}.",
        "unsourced": ("The last sentence is not in the help-center article; an unsupported claim fails grounded." if strict
                      else "Answers the question with every required fact; the closing line is harmless."),
        "quoted": ("The screenshot detail is in the help-center article, so the reply is grounded." if strict
                   else "Mentions screenshots the facts list does not carry; reads as invented."),
    }
    return {"verdict": "pass" if grounded and answers else "fail", "answers": answers, "grounded": grounded,
            "tone": tone, "why": reasons[flaw]}


def yesno(v):
    return "yes" if v else "no"


def score(x):
    return max(0, min(10, round(x)))


def item(case, rep, text, covered, rubric, judged, quality, rng):
    cid, split, title, ask, facts, _ = case
    confidence = round(rng.uniform(0.7, 0.99), 2)
    return {
        "id": cid,
        "rep": rep,
        "group": split,
        "title": title,
        "short": title,
        "subtitle": f"rep {rep} · {len(text.split())} words",
        "meta": f"judge rubric-{rubric} · confidence {confidence:.2f}",
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
            "verdict": {"value": judged["verdict"], "why": f"Judge, confidence {confidence:.2f}: {judged['why']}"},
            "answers": {"value": yesno(judged["answers"])},
            "grounded": {"value": yesno(judged["grounded"])},
            "tone": {"value": yesno(judged["tone"])},
            "quality": {"value": quality, "why": judged["why"]},
        },
        "values": {
            "words": len(text.split()),
            "judge_cost_usd": round(rng.uniform(0.8, 1.2) * JUDGE_COST[rubric], 4),
        },
    }


def main():
    rng = random.Random(11)
    flaws = [f for f, n in FLAW_COUNTS.items() for _ in range(n)]
    rng.shuffle(flaws)
    drafts = [(case, rep, flaw, *write_draft(case, flaw, rng))
              for (case, rep), flaw in zip([(c, r) for c in CASES for r in range(2)], flaws)]
    runs = {"v1": [], "v2": []}
    for case, rep, flaw, text, covered in drafts:
        old, new = grade("v1", flaw, case[4], covered), grade("v2", flaw, case[4], covered)
        q_old = score(rng.gauss(8.0 if old["verdict"] == "pass" else 4.6, 1.0))
        shift = 0 if old["verdict"] == new["verdict"] else 3 if new["verdict"] == "pass" else -3
        q_new = score(q_old + shift + rng.choice([-1, 0, 0, 0, 1]))
        runs["v1"].append(item(case, rep, text, covered, "v1", old, q_old, rng))
        row = item(case, rep, text, covered, "v2", new, q_new, rng)
        if old["verdict"] != new["verdict"]:
            row["flag"] = f"Flipped: rubric-v1 said {old['verdict']}"
        runs["v2"].append(row)
    (HERE / "runs").mkdir(exist_ok=True)
    for rubric, rows in runs.items():
        with (HERE / "runs" / f"rubric-{rubric}.jsonl").open("w") as f:
            for r in rows:
                f.write(json.dumps(r) + "\n")
        passed = sum(r["proposed"]["verdict"]["value"] == "pass" for r in rows)
        print(f"rubric-{rubric}  pass {passed}/{len(rows)}  flagged {sum('flag' in r for r in rows)}")


if __name__ == "__main__":
    main()
