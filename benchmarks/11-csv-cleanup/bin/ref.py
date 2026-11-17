#!/usr/bin/env python3
# The reference clean-ups: ref.py <op> < people.csv > out.csv
import csv, sys

def clean(op, rows):
    if op == "trim":
        return [[f.strip() for f in r] for r in rows]
    if op == "lower-email":
        return [[r[0], r[1].lower(), r[2]] for r in rows]
    if op == "drop-no-email":
        return [r for r in rows if r[1].strip()]
    if op == "sort-by-city":
        return sorted(rows, key=lambda r: r[2])
    if op == "dedupe-email":
        seen, out = set(), []
        for r in rows:
            key = r[1].strip().lower()
            if key not in seen:
                seen.add(key)
                out.append(r)
        return out
    raise SystemExit("unknown op " + op)

def run(op, text):
    rows = list(csv.reader(text.splitlines()))
    return [rows[0]] + clean(op, rows[1:])

if __name__ == "__main__":
    csv.writer(sys.stdout, lineterminator="\n").writerows(run(sys.argv[1], sys.stdin.read()))
