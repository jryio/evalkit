#!/usr/bin/env python3
"""Write runs/<run>.jsonl: one graded query per line, for each round of a hill climb.

Usage: python3 build.py

This stands in for hill-climbing an agent that turns analytics questions into SQL.
Each round changes the agent's instructions once and runs it on the same 12 questions,
twice. A grader runs every query on a test database and compares the result with the
expected answer; its verdict goes under `proposed`. Cost and latency are measured, so
they go under `values`. Eight questions are the ones the instructions were tuned on
(train) and four are held out, so round 4, which copies the training answers into the
instructions, scores higher on train and lower on held-out. A query that passed last
round and fails now gets a `flag`. The numbers are synthetic (seeded).
"""
import json
import random
from pathlib import Path

HERE = Path(__file__).resolve().parent

TABLES = """\
- **customers**: id, name, country, signed_up_at
- **orders**: id, customer_id, placed_at (UTC), status (paid, pending or cancelled), total_cents
- **order_items**: order_id, product_id, quantity, price_cents
- **products**: id, name, category
- **refunds**: order_id, amount_cents, refunded_at"""

# Per round: the run, prompt tokens, output tokens. suite.yaml says what each round changed.
ROUNDS = [
    ("baseline", 900, 170),
    ("column-docs", 1600, 175),
    ("examples", 2600, 180),
    ("copied-answers", 5200, 160),
    ("no-copies", 2900, 185),
    ("row-check", 3000, 320),
]

# id, group, question, the first round it passes (None: never), the round it breaks
# (None: never), the right query, the wrong one, and why the wrong one fails.
QUESTIONS = [
    ("orders-last-week", "train", "How many paid orders were placed last week?", 1, None,
     """SELECT count(*)
FROM orders
WHERE status = 'paid'
  AND placed_at >= date_trunc('week', now()) - interval '7 days'
  AND placed_at <  date_trunc('week', now());""",
     """SELECT count(*)
FROM orders
WHERE status = 'paid'
  AND placed_at > now() - interval '7 days';""",
     "It counts the last seven days instead of last week, so it mixes in orders from this week."),
    ("revenue-after-refunds", "train", "What was revenue by country in October, after refunds?", 2, None,
     """SELECT c.country,
       sum(o.total_cents - coalesce(r.refunded, 0)) / 100.0 AS revenue
FROM orders o
JOIN customers c ON c.id = o.customer_id
LEFT JOIN (
  SELECT order_id, sum(amount_cents) AS refunded
  FROM refunds
  GROUP BY order_id
) r ON r.order_id = o.id
WHERE o.status = 'paid'
  AND o.placed_at >= '2026-10-01' AND o.placed_at < '2026-11-01'
GROUP BY c.country
ORDER BY revenue DESC;""",
     """SELECT c.country,
       sum(o.total_cents) / 100.0 AS revenue
FROM orders o
JOIN customers c ON c.id = o.customer_id
WHERE o.status = 'paid'
  AND o.placed_at >= '2026-10-01' AND o.placed_at < '2026-11-01'
GROUP BY c.country
ORDER BY revenue DESC;""",
     "It never subtracts refunds, so every country's revenue comes out too high."),
    ("top-products", "train", "Which five products sold the most units this year?", 1, None,
     """SELECT p.name, sum(i.quantity) AS units
FROM order_items i
JOIN orders o ON o.id = i.order_id
JOIN products p ON p.id = i.product_id
WHERE o.status = 'paid'
  AND o.placed_at >= date_trunc('year', now())
GROUP BY p.name
ORDER BY units DESC
LIMIT 5;""",
     """SELECT p.name, count(*) AS units
FROM order_items i
JOIN orders o ON o.id = i.order_id
JOIN products p ON p.id = i.product_id
WHERE o.status = 'paid'
  AND o.placed_at >= date_trunc('year', now())
GROUP BY p.name
ORDER BY units DESC
LIMIT 5;""",
     "It counts orders instead of units, so a product people buy in bulk ranks too low."),
    ("both-months", "train", "Which customers ordered in both September and October?", 2, None,
     """SELECT customer_id FROM orders
WHERE placed_at >= '2026-09-01' AND placed_at < '2026-10-01'
INTERSECT
SELECT customer_id FROM orders
WHERE placed_at >= '2026-10-01' AND placed_at < '2026-11-01';""",
     """SELECT DISTINCT customer_id
FROM orders
WHERE placed_at >= '2026-09-01' AND placed_at < '2026-11-01';""",
     "It returns customers who ordered in either month, not in both."),
    ("refund-share", "train", "What share of paid orders were refunded?", 1, None,
     """SELECT count(DISTINCT r.order_id)::numeric
       / count(DISTINCT o.id) AS share
FROM orders o
LEFT JOIN refunds r ON r.order_id = o.id
WHERE o.status = 'paid';""",
     """SELECT count(DISTINCT r.order_id)
       / count(DISTINCT o.id) AS share
FROM orders o
LEFT JOIN refunds r ON r.order_id = o.id
WHERE o.status = 'paid';""",
     "It divides one whole number by another, so the share always comes out as 0."),
    ("daily-buyers", "train", "How many customers bought something on each of the last 14 days?", 4, None,
     """SELECT d::date AS day, count(DISTINCT o.customer_id) AS buyers
FROM generate_series(current_date - 13, current_date, interval '1 day') AS d
LEFT JOIN orders o
  ON o.placed_at >= d AND o.placed_at < d + interval '1 day'
 AND o.status = 'paid'
GROUP BY d
ORDER BY d;""",
     """SELECT placed_at::date AS day, count(DISTINCT customer_id) AS buyers
FROM orders
WHERE status = 'paid'
  AND placed_at >= current_date - 13
GROUP BY day
ORDER BY day;""",
     "Days with no sales are missing from the result instead of showing 0."),
    ("median-basket", "train", "What is the median number of items in an order?", 3, None,
     """SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY items) AS median
FROM (
  SELECT order_id, sum(quantity) AS items
  FROM order_items
  GROUP BY order_id
) t;""",
     """SELECT avg(items) AS median
FROM (
  SELECT order_id, sum(quantity) AS items
  FROM order_items
  GROUP BY order_id
) t;""",
     "It returns the average, which a few very large orders pull far above the median."),
    ("stuck-pending", "train", "Which orders have been pending for more than 48 hours?", 1, None,
     """SELECT id, placed_at
FROM orders
WHERE status = 'pending'
  AND placed_at < now() - interval '48 hours'
ORDER BY placed_at;""",
     """SELECT id, placed_at
FROM orders
WHERE status = 'pending'
  AND placed_at > now() - interval '48 hours'
ORDER BY placed_at;""",
     "It finds orders placed in the last 48 hours, the opposite of what was asked."),
    ("first-order-value", "test", "What is the average value of a customer's first order?", 3, 4,
     """SELECT avg(total_cents) / 100.0 AS first_order_value
FROM (
  SELECT DISTINCT ON (customer_id) total_cents
  FROM orders
  WHERE status = 'paid'
  ORDER BY customer_id, placed_at
) first_orders;""",
     """SELECT avg(o.total_cents) / 100.0 AS first_order_value
FROM orders o
JOIN customers c ON c.id = o.customer_id
WHERE o.status = 'paid'
  AND o.placed_at < c.signed_up_at + interval '30 days';""",
     "It averages every order from a customer's first 30 days, not only their first order."),
    ("repeat-rate", "test", "For each signup month, what share of customers ordered more than once?", 6, None,
     """SELECT date_trunc('month', c.signed_up_at) AS cohort,
       avg((coalesce(o.n, 0) > 1)::int) AS repeat_rate
FROM customers c
LEFT JOIN (
  SELECT customer_id, count(*) AS n
  FROM orders
  WHERE status = 'paid'
  GROUP BY customer_id
) o ON o.customer_id = c.id
GROUP BY cohort
ORDER BY cohort;""",
     """SELECT date_trunc('month', c.signed_up_at) AS cohort,
       avg((o.n > 1)::int) AS repeat_rate
FROM customers c
JOIN (
  SELECT customer_id, count(*) AS n
  FROM orders
  WHERE status = 'paid'
  GROUP BY customer_id
) o ON o.customer_id = c.id
GROUP BY cohort
ORDER BY cohort;""",
     "It leaves out customers who never ordered, so every month's repeat rate looks too high."),
    ("sunday-weeks", "test", "What was revenue per week this quarter, with weeks that start on Sunday?", None, None,
     """SELECT date_trunc('week', placed_at + interval '1 day') - interval '1 day' AS week,
       sum(total_cents) / 100.0 AS revenue
FROM orders
WHERE status = 'paid'
  AND placed_at >= date_trunc('quarter', now())
GROUP BY week
ORDER BY week;""",
     """SELECT date_trunc('week', placed_at) AS week,
       sum(total_cents) / 100.0 AS revenue
FROM orders
WHERE status = 'paid'
  AND placed_at >= date_trunc('quarter', now())
GROUP BY week
ORDER BY week;""",
     "Postgres weeks start on Monday, so each Sunday's sales land in the wrong week."),
    ("category-growth", "test", "Which product category grew the most from September to October?", 1, 4,
     """WITH monthly AS (
  SELECT p.category,
         date_trunc('month', o.placed_at) AS month,
         sum(i.quantity * i.price_cents) AS revenue
  FROM order_items i
  JOIN orders o ON o.id = i.order_id
  JOIN products p ON p.id = i.product_id
  WHERE o.status = 'paid'
    AND o.placed_at >= '2026-09-01' AND o.placed_at < '2026-11-01'
  GROUP BY 1, 2
)
SELECT category,
       max(revenue) FILTER (WHERE month = '2026-10-01')
     - max(revenue) FILTER (WHERE month = '2026-09-01') AS growth
FROM monthly
GROUP BY category
ORDER BY growth DESC
LIMIT 1;""",
     """WITH monthly AS (
  SELECT p.category,
         date_trunc('month', o.placed_at) AS month,
         sum(i.quantity * i.price_cents) AS revenue
  FROM order_items i
  JOIN orders o ON o.id = i.order_id
  JOIN products p ON p.id = i.product_id
  WHERE o.placed_at >= '2026-09-01' AND o.placed_at < '2026-11-01'
  GROUP BY 1, 2
)
SELECT category,
       max(revenue) FILTER (WHERE month = '2026-10-01')
     - max(revenue) FILTER (WHERE month = '2026-09-01') AS growth
FROM monthly
GROUP BY category
ORDER BY growth DESC
LIMIT 1;""",
     "It counts cancelled orders, so a category with many cancellations looks like it grew."),
]

PRICE_IN, PRICE_OUT = 3.0, 15.0  # $ per million tokens


def passes(question, n, rep, rng):
    first, breaks = question[3], question[4]
    if first is None or n < first or n == breaks:
        return False
    # In the round a fix lands, the second try still misses half the time.
    return not (n == first and rep == 1 and rng.random() < 0.5)


def row(question, n, rep, passed, before, rng):
    qid, group, ask, _, _, right, wrong, why = question
    prompt_tokens, out_mean = ROUNDS[n - 1][1], ROUNDS[n - 1][2]
    out_tokens = max(60, round(rng.gauss(out_mean, out_mean * 0.2)))
    sql = right if passed else wrong
    show = [{"id": "query", "kind": "code", "label": "Query the agent wrote", "language": "sql", "code": sql}]
    if before is not None and before != passed:
        show.append({
            "id": "change", "kind": "compare", "language": "sql", "below": True,
            "panes": [
                {"label": f"Round {n - 1}", "note": "passed" if before else "failed", "tone": "positive" if before else "negative", "code": right if before else wrong},
                {"label": f"Round {n}", "note": "passed" if passed else "failed", "tone": "positive" if passed else "negative", "code": sql},
            ],
        })
    item = {
        "id": qid,
        "rep": rep,
        "group": group,
        "title": ask,
        "short": ask,
        "subtitle": f"try {rep + 1}",
        "meta": f"Round {n} · {'held out' if group == 'test' else 'tuned on'}",
        "show": show,
        "context": [{"kind": "markdown", "label": "Tables", "text": TABLES}],
        "proposed": {"verdict": {
            "value": "pass" if passed else "fail",
            "why": "The grader ran the query and got the expected answer." if passed else f"The grader ran the query and got a different answer. {why}",
        }},
        "values": {
            "cost_usd": round((prompt_tokens * PRICE_IN + out_tokens * PRICE_OUT) / 1e6, 5),
            "latency_s": round(max(0.8, 1.2 + out_tokens / 120 + prompt_tokens / 4000 + rng.gauss(0, 0.3)), 1),
        },
    }
    if before and not passed:
        item["flag"] = f"Passed in round {n - 1}, fails now"
    return item


def main():
    rng = random.Random(12)
    (HERE / "runs").mkdir(exist_ok=True)
    before = {}
    for n in range(1, len(ROUNDS) + 1):
        rows = []
        for q in QUESTIONS:
            for rep in range(2):
                passed = passes(q, n, rep, rng)
                rows.append(row(q, n, rep, passed, before.get((q[0], rep)), rng))
                before[(q[0], rep)] = passed
        with (HERE / "runs" / f"{ROUNDS[n - 1][0]}.jsonl").open("w") as f:
            for r in rows:
                f.write(json.dumps(r) + "\n")
        score = lambda g: sum(r["proposed"]["verdict"]["value"] == "pass" for r in rows if g in (None, r["group"]))
        cost = sum(r["values"]["cost_usd"] for r in rows) / len(rows)
        print(f"round {n} {ROUNDS[n - 1][0]:15} pass {score(None)}/24  train {score('train')}/16  held-out {score('test')}/8  cost ${cost:.4f}  flagged {sum('flag' in r for r in rows)}")


if __name__ == "__main__":
    main()
