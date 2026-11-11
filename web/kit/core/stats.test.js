import { test } from "node:test";
import assert from "node:assert/strict";
import { bootstrapMean, mcnemar, quantile, rng, wilson } from "./stats.js";

const round = (v, d = 4) => Number(v.toFixed(d));

test("wilson matches the reference interval and stays inside [0, 1]", () => {
  const w = wilson(8, 10);
  assert.deepEqual([w.value, round(w.lo), round(w.hi)], [0.8, 0.4902, 0.9433]);
  assert.equal(wilson(0, 10).lo, 0);
  assert.equal(wilson(10, 10).hi, 1);
  assert.equal(wilson(0, 0), null);
});

test("rng is deterministic per seed and in [0, 1)", () => {
  const draw = seed => Array.from({ length: 5 }, rng(seed));
  assert.deepEqual(draw(7), draw(7));
  assert.notDeepEqual(draw(7), draw(8));
  assert.ok(draw(1).every(x => x >= 0 && x < 1));
});

test("quantile interpolates linearly (type 7)", () => {
  assert.equal(quantile([1, 2, 3, 4], 0.5), 2.5);
  assert.equal(quantile([1, 2, 3, 4], 0), 1);
  assert.equal(quantile([1, 2, 3, 4], 1), 4);
  assert.equal(quantile([10, 20], 0.25), 12.5);
  assert.equal(quantile([5], 0.975), 5);
});

test("bootstrapMean is deterministic for a seed and brackets the mean", () => {
  const xs = [3, 5, 4, 8, 6, 2, 7, 5, 4, 6];
  const a = bootstrapMean(xs, { seed: 3 });
  assert.deepEqual(a, bootstrapMean(xs, { seed: 3 }));
  assert.equal(a.value, 5);
  assert.ok(a.lo < a.value && a.value < a.hi);
  assert.equal(bootstrapMean([]), null);
});

test("bootstrapMean with clusters resamples whole cases", () => {
  // Two cases, ten reps each: one always 1, one always 0. Only two independent units exist.
  const xs = [...Array(10).fill(1), ...Array(10).fill(0)];
  const clusters = [...Array(10).fill("a"), ...Array(10).fill("b")];
  const flat = bootstrapMean(xs);
  const kept = bootstrapMean(xs, { clusters });
  assert.equal(kept.value, 0.5);
  assert.deepEqual([kept.lo, kept.hi], [0, 1]);
  assert.ok(flat.lo > 0.2 && flat.hi < 0.8);
});

test("mcnemar is the exact two-sided binomial test", () => {
  assert.equal(round(mcnemar(10, 2)), 0.0386);
  assert.equal(mcnemar(2, 10), mcnemar(10, 2));
  assert.equal(mcnemar(0, 0), 1);
  assert.equal(mcnemar(3, 3), 1);
  assert.equal(round(mcnemar(0, 5)), 0.0625);
  assert.ok(mcnemar(0, 2000) < 1e-10);
  assert.equal(mcnemar(1000, 1000), 1);
});
