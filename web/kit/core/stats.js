// Small statistics for comparing runs: a rate interval, a seeded bootstrap and a paired test.

const mean = xs => xs.reduce((a, b) => a + b, 0) / xs.length;

// wilson is the Wilson score interval for k successes in n trials.
export function wilson(k, n, z = 1.96) {
  if (!n) return null;
  const p = k / n, z2 = z * z, d = 1 + z2 / n;
  const mid = (p + z2 / (2 * n)) / d;
  const half = (z / d) * Math.sqrt((p * (1 - p)) / n + z2 / (4 * n * n));
  return { value: p, lo: Math.max(0, mid - half), hi: Math.min(1, mid + half) };
}

// rng is mulberry32: a fast, seeded generator, so intervals do not jitter between renders.
export function rng(seed) {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = Math.imul(a ^ (a >>> 15), a | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

// quantile interpolates linearly between order statistics (R's type 7).
export function quantile(sorted, q) {
  const h = (sorted.length - 1) * q;
  const lo = Math.floor(h);
  return lo + 1 < sorted.length ? sorted[lo] + (h - lo) * (sorted[lo + 1] - sorted[lo]) : sorted[lo];
}

// bootstrapMean is a percentile interval for the mean. With clusters (a key per x), it
// resamples whole clusters, so the reps of one case move together.
export function bootstrapMean(xs, { B = 2000, seed = 1, clusters } = {}) {
  if (!xs.length) return null;
  const units = clusters ? [...groupBy(xs, clusters).values()] : xs.map(x => [x]);
  const next = rng(seed);
  const means = [];
  for (let b = 0; b < B; b++) {
    let sum = 0, count = 0;
    for (let k = 0; k < units.length; k++) {
      const u = units[Math.floor(next() * units.length)];
      for (const x of u) sum += x;
      count += u.length;
    }
    means.push(sum / count);
  }
  means.sort((a, b) => a - b);
  return { value: mean(xs), lo: quantile(means, 0.025), hi: quantile(means, 0.975) };
}

function groupBy(xs, keys) {
  const out = new Map();
  xs.forEach((x, i) => {
    const g = out.get(keys[i]);
    if (g) g.push(x);
    else out.set(keys[i], [x]);
  });
  return out;
}

// mcnemar is the exact two-sided test on discordant pairs: Binomial(wins + losses, 0.5).
export function mcnemar(wins, losses) {
  const n = wins + losses;
  if (!n) return 1;
  let logTerm = -n * Math.LN2, tail = 0;
  for (let i = 0; i <= Math.min(wins, losses); i++) {
    tail += Math.exp(logTerm);
    logTerm += Math.log((n - i) / (i + 1));
  }
  return Math.min(1, 2 * tail);
}
