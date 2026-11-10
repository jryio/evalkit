// Number formatting shared by metrics, charts and the items table.

const MINUS = "−";
const PREFIX_UNITS = new Set(["$", "€", "£", "¥"]);

// fmtNumber prints d decimals, or up to three when d is undefined.
export const fmtNumber = (v, d) =>
  v.toLocaleString("en-US", d === undefined ? { maximumFractionDigits: 3 } : { minimumFractionDigits: d, maximumFractionDigits: d }).replace("-", MINUS);

// signed prints a difference: "+0.12", "−3", "±0".
export const signed = (v, d) => (v === 0 ? "±0" : `${v > 0 ? "+" : ""}${fmtNumber(v, d)}`);

// withUnit appends a unit ("42%", "42 ms") or prefixes a currency ("$0.22").
export function withUnit(text, unit) {
  if (!unit) return text;
  if (PREFIX_UNITS.has(unit)) return text.startsWith(MINUS) ? `${MINUS}${unit}${text.slice(1)}` : `${unit}${text}`;
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
