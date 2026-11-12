#!/usr/bin/env python3
"""Write runs/<model>-<effort>.jsonl: one judged support-reply draft per line, per configuration.

Usage: python3 build.py

This stands in for a model sweep: three models at four effort levels each draft a
reply to the 16 support cases of demo 08, twice. One LLM judge grades every draft
with a verdict and a 0-10 quality score; those go under `proposed`. Cost, output
tokens and latency are measured, so they go under `values`. A line keeps the case id
and sets `rep`, so the same case and rep pair across the twelve runs. Rows the judge
was unsure about get a `flag`. The numbers are synthetic (seeded); the models and
Acme are fictional.
"""
import json
import random
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.dont_write_bytecode = True
sys.path.insert(0, str(HERE.parent / "08-judge-round"))
from build import CASES, draft  # noqa: E402

# $ per million input and output tokens, output tokens per second, judge quality of a passing draft
MODELS = {
    "lark": {"price": (0.5, 2.5), "speed": 210, "quality": 6.6},
    "heron": {"price": (1.5, 7.5), "speed": 110, "quality": 7.5},
    "osprey": {"price": (5.0, 25.0), "speed": 60, "quality": 8.3},
}
# Per model and effort: drafts of the 32 that pass, mean output tokens
SWEEP = {
    "lark": {"low": (14, 350), "medium": (17, 600), "high": (19, 1100), "max": (20, 1900)},
    "heron": {"low": (19, 450), "medium": (22, 800), "high": (26, 1500), "max": (27, 2600)},
    "osprey": {"low": (23, 500), "medium": (26, 900), "high": (29, 1700), "max": (29, 4800)},
}
EFFORT_QUALITY = {"low": -0.5, "medium": 0, "high": 0.4, "max": 0.5}


def reason(flaw, facts, covered):
    return {
        None: "Answers the question with every required fact, in a plain, friendly voice.",
        "invented": "Offers a credit nobody approved; that is an invented commitment.",
        "missed": f"Stops after the first step and leaves out: {', '.join(facts[len(covered):])}.",
        "stiff": "Facts are right, but the 'Dear Customer' opener reads like a form letter.",
        "padded": "Facts are right, but the closing paragraph is filler.",
    }[flaw]


def row(model, effort, case, rep, passes, in_tokens, rng):
    cid, split, title, ask, facts, _ = case
    spec = MODELS[model]
    flaw = rng.choice(["invented", "missed"]) if not passes else rng.choices([None, "stiff", "padded"], weights=[80, 10, 10])[0]
    text, covered = draft(case, flaw, rng)
    mean = spec["quality"] + EFFORT_QUALITY[effort] - (1.5 if flaw else 0)
    quality = max(0, min(10, round(rng.gauss(mean if passes else 4.2, 0.9 if passes else 1.2))))
    confidence = round(rng.uniform(0.75, 0.99) if passes else rng.uniform(0.55, 0.97), 2)
    out_tokens = max(80, round(rng.gauss(SWEEP[model][effort][1], SWEEP[model][effort][1] * 0.22)))
    price_in, price_out = spec["price"]
    why = reason(flaw, facts, covered)
    item = {
        "id": cid,
        "rep": rep,
        "group": split,
        "title": title,
        "short": title,
        "subtitle": f"rep {rep} · {len(text.split())} words",
        "meta": f"{model} · {effort} effort · judge confidence {confidence:.2f}",
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
            "verdict": {"value": "pass" if passes else "fail", "why": f"Judge, confidence {confidence:.2f}: {why}"},
            "quality": {"value": quality, "why": why},
        },
        "values": {
            "cost_usd": round((in_tokens * price_in + out_tokens * price_out) / 1e6, 5),
            "out_tokens": out_tokens,
            "latency_s": round(max(0.8, 0.6 + out_tokens / spec["speed"] + rng.gauss(0, 0.4)), 1),
        },
    }
    if confidence < 0.62:
        item["flag"] = f"Judge confidence {confidence:.2f}"
    return item


def main():
    rng = random.Random(9)
    # Hard cases are hard for every model: a draft passes when its difficulty, plus a
    # little per-run noise, is among the run's easiest.
    difficulty = {(case[0], rep): rng.random() for case in CASES for rep in range(2)}
    in_tokens = {case[0]: rng.randint(1050, 1400) for case in CASES}
    (HERE / "runs").mkdir(exist_ok=True)
    for model, efforts in SWEEP.items():
        for effort, (n_pass, _) in efforts.items():
            passing = set(sorted(difficulty, key=lambda k: difficulty[k] + rng.gauss(0, 0.15))[:n_pass])
            rows = [row(model, effort, case, rep, (case[0], rep) in passing, in_tokens[case[0]], rng)
                    for case in CASES for rep in range(2)]
            run = f"{model}-{effort}"
            with (HERE / "runs" / f"{run}.jsonl").open("w") as f:
                for r in rows:
                    f.write(json.dumps(r) + "\n")
            cost = sum(r["values"]["cost_usd"] for r in rows) / len(rows)
            print(f"{run:15} pass {n_pass / len(rows):.2f}  cost ${cost:.4f}  flagged {sum('flag' in r for r in rows)}")


if __name__ == "__main__":
    main()
