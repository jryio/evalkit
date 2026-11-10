#!/usr/bin/env python3
"""Build suite.yaml from a status-update skill's run files in runs/.

Usage: python3 build.py   (writes suite.yaml next to this file)

This is the adapter a skill writes once: it maps its own run format onto the
evalkit schema. Two live drafts are already posted to the tracker; the rest
wait for approval so the approve-for-posting flow can be tried.
"""
import json
from datetime import datetime
from pathlib import Path

import yaml

HERE = Path(__file__).resolve().parent
KEEP_POSTED = {"live-mobile-checkout", "live-soc2-audit"}
SHORT = {"Data warehouse migration": "Warehouse migration"}
HEALTH = {"onTrack": ("On track", "positive"), "atRisk": ("At risk", "warning"), "offTrack": ("Off track", "negative")}

QUESTIONS = [
    {"id": "health", "kind": "choice", "label": "Health", "key": "h", "status": True,
     "options": [{"value": v, "label": label, "tone": tone} for v, (label, tone) in HEALTH.items()]},
    {"id": "verdict", "kind": "choice", "label": "Verdict", "primary": True, "options": [
        {"value": "good", "label": "Good", "tone": "positive", "icon": "check"},
        {"value": "close", "label": "Close", "tone": "warning", "icon": "approx"},
        {"value": "off", "label": "Off", "tone": "negative", "icon": "cross"}]},
    {"id": "flags", "kind": "multi", "label": "Flags", "with": "verdict", "options": [
        {"value": "picks", "label": "Wrong picks", "key": "p"},
        {"value": "voice", "label": "Not my voice", "key": "v"}]},
    {"id": "post", "kind": "action", "label": "Post", "key": "a", "groups": ["live"],
     "summary": "Claude posts these to Linear.",
     "states": {
         "off": {"button": "Approve for posting", "note": "Nothing posts until you approve"},
         "on": {"button": "Approved", "note": "Posts to Linear when you send", "badge": "Post"},
         "done": {"button": "Posted to Linear", "note": "edit it in Linear from here on", "badge": "Posted"}}},
    {"id": "notes", "kind": "text", "label": "Notes for Claude", "key": "n",
     "placeholder": "What would you change? What's missing? How would you say it?"},
    {"id": "should_be_in", "kind": "toggle", "label": "Should be in", "each": "evidence"},
]


def day(iso):
    return datetime.fromisoformat(iso.replace("Z", "+00:00")).strftime("%b %-d")


def item(run):
    live = run.get("mode") != "blind"
    days = run.get("days_since_last")
    meta = f"{day(run['window_start'])} → {day(run['window_end'])}"
    if days is not None:
        meta += f" · {days} days since the {'last' if live else 'previous'} update"
    actual = run.get("actual")

    draft = {"id": "draft", "kind": "text", "label": "Draft", "editable": True, "text": run["draft"],
             "limit": {"target": [200, 300], "max": 500}}
    show = [draft]
    if actual:
        draft["width"] = "half"
        label, tone = HEALTH.get(actual.get("health"), (actual.get("health"), "neutral"))
        show.append({"id": "actual", "kind": "text", "label": "You wrote", "text": actual.get("body") or "",
                     "width": "half", "status": {"label": label, "tone": tone}})

    outcomes = sorted(run.get("outcomes") or [], key=lambda o: (not o["included"], o.get("rank") or 99))
    used = sum(o["included"] for o in outcomes)
    rows = []
    for o in outcomes:
        row = {"id": o["id"], "title": o["title"]}
        if o["included"]:
            row.update(badge={"label": "Used", "tone": "positive"}, ask=False)
        elif o.get("cut_reason"):
            row["detail"] = o["cut_reason"]
        links = [{"label": f"{s['type']}: {s['label']}", **({"url": s["url"]} if s.get("url") else {})} for s in o.get("sources") or []]
        if links:
            row["links"] = links
        rows.append(row)
    context = [{"id": "evidence", "kind": "list", "label": "Why these bullets", "note": f"{used} used · {len(outcomes) - used} cut", "rows": rows}]
    if run.get("previous_update"):
        context.append({"kind": "text", "label": "Previous update", "text": run["previous_update"]})
    sources = run.get("sources_checked") or []
    context.append({"kind": "table", "label": "Sources checked", "note": str(len(sources)),
                    "columns": [{"key": "source", "label": "Source"}, {"key": "status", "label": "Status"},
                                {"key": "items", "label": "Items"}, {"key": "notes", "label": "Notes", "muted": True}],
                    "rows": [{"source": s["source"], "status": {"value": s["status"], "tone": "negative"} if s["status"] == "failed" else s["status"],
                              "items": s.get("items", ""), "notes": s.get("notes", "")} for s in sources]})

    out = {
        "id": run["slug"],
        "group": "live" if live else "backtest",
        "title": run["project"],
        **({"short": SHORT[run["project"]]} if run["project"] in SHORT else {}),
        "subtitle": f"{days if days is not None else '?'} days since last update" if live else day(run["window_end"]),
        "badges": [{"label": "Live draft", "tone": "positive"} if live else {"label": "Back-test", "tone": "warning"}],
        "meta": meta,
        "proposed": {"health": {"value": run.get("health", "onTrack"), "why": run.get("health_reason", "")}},
        "show": show,
    }
    if run.get("questions"):
        out["ask"] = [{"id": f"q{i + 1}", "kind": "text", "label": q} for i, q in enumerate(run["questions"])]
    out["context"] = context
    if run.get("posted") and run["slug"] in KEEP_POSTED:
        out["done"] = {"post": {"at": run["posted"]["at"], "url": run["posted"]["url"]}}
    return out


def main():
    runs = [json.loads(p.read_text()) for p in sorted((HERE / "runs").glob("*.json"))]
    runs.sort(key=lambda r: (r.get("mode") == "blind", r["window_end"] if r.get("mode") == "blind" else r["project"]))
    suite = {
        "title": "Project status updates",
        "description": "Review each draft update: approve the live ones to post, compare back-tests with what the owner wrote.",
        "send": "Send to Claude",
        "reviewed": ["verdict", "post"],
        "groups": [{"id": "live", "label": "Live drafts"}, {"id": "backtest", "label": "Back-tests"}],
        "questions": QUESTIONS,
        "items": [item(r) for r in runs],
    }
    text = yaml.safe_dump(suite, sort_keys=False, allow_unicode=True, width=120)
    (HERE / "suite.yaml").write_text("# Generated by build.py from runs/*.json. Edit build.py, not this file.\n" + text)
    print(HERE / "suite.yaml")


if __name__ == "__main__":
    main()
