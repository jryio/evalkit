// Metrics against an optional baseline, as tiles (default) or a table. The change is
// toned by whether it moves in the `better` direction beyond the tolerance.
import { html } from "../html.js";
import { cx, tone } from "../ui/bits.js";
import { autoDecimals, fmtNumber as fmt, withUnit } from "../core/format.js";

const MINUS = "−";

const decimals = m => m.decimals ?? autoDecimals([m.value, m.baseline ?? 0]);

// change returns { pct, tone } against the baseline, or null without one.
function change(m) {
  if (m.baseline == null) return null;
  const pct = m.baseline === 0 ? null : ((m.value - m.baseline) / Math.abs(m.baseline)) * 100;
  if (pct === null || !m.better || Math.abs(pct) <= (m.tolerance ?? 1)) return { pct, tone: "neutral" };
  const improved = m.better === "lower" ? pct < 0 : pct > 0;
  return { pct, tone: improved ? "positive" : "negative" };
}

const fmtPct = pct => (pct === null ? "" : `${pct > 0 ? "+" : pct < 0 ? MINUS : ""}${Math.abs(pct).toFixed(1)}%`);

export default function MetricsArtifact({ a }) {
  return a.layout === "table" ? html`<${MetricsTable} a=${a} />` : html`<${MetricsTiles} a=${a} />`;
}

function MetricsTiles({ a }) {
  return html`<div class="metric-tiles">${a.metrics.map(m => {
    const c = change(m);
    const d = decimals(m);
    return html`<div class="metric-tile">
      <div class="metric-label">${m.label}</div>
      <div class="metric-value">${fmt(m.value, d)}${m.unit ? html`<span class="metric-unit">${m.unit}</span>` : null}</div>
      ${c ? html`<div class=${cx("metric-delta", tone(c.tone))}>${[fmtPct(c.pct), `vs ${withUnit(fmt(m.baseline, d), m.unit)}`].filter(Boolean).join(" ")}</div>` : null}
    </div>`;
  })}</div>`;
}

function MetricsTable({ a }) {
  return html`<table class="metric-table">
    <thead><tr>
      <th>Metric</th>
      <th class="right">${a.columns?.baseline ?? "Baseline"}</th>
      <th class="right">${a.columns?.value ?? "Value"}</th>
      <th class="right">Change</th>
    </tr></thead>
    <tbody>${a.metrics.map(m => {
      const c = change(m);
      const d = decimals(m);
      return html`<tr>
        <td>${m.label}</td>
        <td class="right">${m.baseline == null ? "" : withUnit(fmt(m.baseline, d), m.unit)}</td>
        <td class="right">${withUnit(fmt(m.value, d), m.unit)}</td>
        <td class=${cx("right", c && "toned", c && tone(c.tone))}>${c ? fmtPct(c.pct) : ""}</td>
      </tr>`;
    })}</tbody>
  </table>`;
}
