// Number formatting shared by metrics, charts and the items table.

const MINUS = "−";
const PREFIX_UNITS = new Set(["$", "€", "£", "¥"]);

// fmtNumber prints d decimals, or up to three when d is undefined.
export const fmtNumber = (v, d) =>
  v.toLocaleString("en-US", d === undefined ? { maximumFractionDigits: 3 } : { minimumFractionDigits: d, maximumFractionDigits: d }).replace("-", MINUS);

// signed prints a difference: "+0.12", "−3", "±0". A change too small for d decimals gets
// more, up to d + 3, so it never reads as zero: "−0.004" at d = 2.
export function signed(v, d) {
  if (v === 0) return "±0";
  for (const max = d + 3; d < max && Number(Math.abs(v).toFixed(d)) === 0; ) d++;
  return `${v > 0 ? "+" : ""}${fmtNumber(v, d)}`;
}

// withUnit appends a unit ("42%", "42 ms") or prefixes a currency after any sign ("$0.22", "+$0.10").
export function withUnit(text, unit) {
  if (!unit) return text;
  const sign = /^[+±−]/.test(text) ? text[0] : "";
  if (PREFIX_UNITS.has(unit)) return `${sign}${unit}${text.slice(sign.length)}`;
  return unit === "%" ? `${text}%` : `${text} ${unit}`;
}

// autoDecimals: whole numbers print without decimals; otherwise large values get none, small ones two.
export function autoDecimals(values) {
  const nums = values.filter(v => typeof v === "number");
  if (nums.every(Number.isInteger)) return 0;
  return Math.max(...nums.map(Math.abs)) >= 100 ? 0 : 2;
}

// stepDecimals is how many decimals a step implies: 0.05 -> 2, 1 -> 0.
export const stepDecimals = step => (step === undefined ? undefined : String(step).split(".")[1]?.length ?? 0);

// deltaTone tones a difference by whether it moves the `better` way by more than `noise`.
export function deltaTone(delta, { better, noise = 0 } = {}) {
  if (!better || Math.abs(delta) <= noise || delta === 0) return "neutral";
  return (better === "lower" ? delta < 0 : delta > 0) ? "positive" : "negative";
}
