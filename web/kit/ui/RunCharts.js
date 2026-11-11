// The Runs view's charts, drawn with Observable Plot. RunTrends is the top row (the
// score run by run, and the score against spend); RunPairs is the selected run against
// its comparison, item by item.
//
// Props, shared by both:
//   suite     the full suite (dimensions, groups, runs)
//   state     the review state; judgeOnly and group are the view's filters
//   rows      summarize(suite, state, { judgeOnly, group }) in run order
//   metric    metricOf(suite); spend: spendOf(suite) or null
//   runs      runList(suite)
//   selected  the selected run id; vs: the run it is compared with (a runs entry), or null
//   select    (runId) => void: select a run
//   open      (runId, { pair, direction }) => void: open the Table on runId, with the
//             item runId/pair selected, or filtered to items that went "up" | "same" | "down"
import * as Plot from "plot";
import { html, useEffect, useMemo, useRef, useState } from "../html.js";
import { autoDecimals, deltaTone, fmtNumber, withUnit } from "../core/format.js";
import { frontier, itemValue, markedBaseline, pairsOf, summarize, vsOf } from "../core/runs.js";
import { cx, fmtDate, tone } from "./bits.js";

const isNum = v => typeof v === "number" && Number.isFinite(v);
const ISO_DATE = /^\d{4}-\d\d-\d\d/;
const HEIGHT = 260;
const STYLE = { background: "transparent", color: "var(--secondary)", fontFamily: "var(--font)", fontSize: "11px", fontVariantNumeric: "tabular-nums", overflow: "visible" };
const TIP = { fill: "var(--surface)", stroke: "var(--separator)", fontSize: 12, lineHeight: 1.35, pathFilter: "none", textPadding: 8 };
const DOT = { r: 3.5, stroke: "var(--surface)", strokeWidth: 1.5 };
const LINE = { strokeWidth: 1.5, strokeLinejoin: "round", strokeLinecap: "round" };
const CHAR = 6.2;
// The top row's two charts share margins, so their plot areas line up.
const MARGIN = { marginTop: 10, marginRight: 16, marginBottom: 36, marginLeft: 40 };
const X_LABEL = { labelAnchor: "center", labelArrow: "none", labelOffset: 30 };

// Series are told apart by shade, then dash; past three, only the selected one takes a hue.
const SHADES = [
  { color: "var(--text)", opacity: 0.8 },
  { color: "var(--tertiary)", opacity: 1 },
  { color: "var(--tertiary)", opacity: 1, dash: "4 3" },
  { color: "var(--tertiary)", opacity: 1, dash: "1 3" },
];
const PLAIN = SHADES[1];
const FADED = { color: "var(--tertiary)", opacity: 0.55 };
const CHOSEN = { color: "var(--accent)", opacity: 1 };

const TONES = ["positive", "neutral", "negative"];
const TONE_FILL = { positive: "var(--positive)", neutral: "var(--neutral)", negative: "var(--negative)" };
const DIRECTION = { positive: "up", neutral: "same", negative: "down" };

// moves are the paired counts' glyphs and words by tone, as the Runs view's tiles show them:
// a lower-is-better metric improves by going down.
function moves(metric) {
  const lower = metric.better === "lower", rate = metric.kind === "rate";
  return {
    positive: { glyph: lower ? "↓" : "↑", word: rate ? "improved" : lower ? "moved down" : "moved up" },
    neutral: { glyph: "=", word: rate ? "same" : "no movement" },
    negative: { glyph: lower ? "↑" : "↓", word: rate ? "regressed" : lower ? "moved up" : "moved down" },
  };
}

// RunTrends is the top row: the score run by run, and beside it the score against spend.
export function RunTrends({ suite, state, rows, metric, spend, runs, selected, vs, select, judgeOnly = false, group = null }) {
  const axis = useMemo(() => trendAxis(suite, runs), [suite, runs]);
  const groupRows = useMemo(() => {
    const groups = suite.groups || [];
    if (axis.series || group || groups.length < 2 || groups.length > 3) return [];
    return groups.map(g => ({ label: g.label ?? g.id, rows: summarize(suite, state, { judgeOnly, group: g.id }) }));
  }, [suite, state, judgeOnly, group, axis]);
  if (!metric) return null;
  const showTrend = rows.filter(r => r.score && axis.x(r.run) !== undefined).length >= 2;
  const showSpend = !!spend && rows.filter(r => r.score && r.spend).length >= 2;
  if (!showTrend && !showSpend) return null;
  const props = { rows, metric, spend, axis, selected, vs, select };
  return html`<div class=${cx("rc-row", !(showTrend && showSpend) && "single")}>
    ${showTrend ? html`<${Trend} ...${props} groupRows=${groupRows} />` : null}
    ${showSpend ? html`<${SpendChart} ...${props} />` : null}
  </div>`;
}

// RunPairs is the selected run against its comparison, item by item: a scatter for a mean
// metric, a bar per run of what improved, held and regressed for a rate.
export function RunPairs({ suite, state, metric, runs, selected, vs: other, open, judgeOnly = false, group = null }) {
  const run = runs.find(r => r.id === selected);
  // A rate draws a bar for every run against its own comparison; a mean only the selected run.
  const bars = useMemo(() => {
    if (!metric || !run || !other) return [];
    const opts = { judgeOnly, group };
    return (metric.kind === "rate" ? runs : [run])
      .map(r => ({ run: r, vs: r.id === run.id ? other : vsOf(suite, runs, r) }))
      .filter(b => b.vs)
      .map(b => ({ ...b, pairs: pairTones(suite, state, metric, runs, b.run.id, b.vs.id, opts) }))
      .filter(b => b.pairs.length)
      .map(b => ({ ...b, counts: tally(b.pairs) }));
  }, [suite, state, metric, runs, selected, other, judgeOnly, group]);
  if (!metric || !run) return null;
  const mine = bars.find(b => b.run.id === selected);
  if (!other || !mine) {
    const shared = other ? pairsOf(suite, run.id, other.id).size : 0;
    const why = !other ? `${run.label} has no run to compare with, so there is nothing to pair.`
      : shared ? `${run.label} shares ${shared} items with ${other.label}, but none has a ${metric.label} value in both.`
      : `${run.label} shares no items with ${other.label}, so there is nothing to pair.`;
    return html`<${Card} title=${other ? `${run.label} vs ${other.label}, item by item` : `${run.label}, item by item`}>
      <p class="rc-empty">${why}</p>
    </${Card}>`;
  }
  const { counts } = mine;
  const rate = metric.kind === "rate";
  const answer = `${run.label} is better on ${counts.positive} of ${mine.pairs.length} items and worse on ${counts.negative}.`;
  return html`<${Card} title=${`${run.label} vs ${other.label}, item by item (${mine.pairs.length} paired)`} answer=${answer}>
    <div class="rc-pairs">
      ${rate
        ? html`<${PairBars} bars=${bars} metric=${metric} selected=${selected} open=${open} />`
        : html`<${PairScatter} suite=${suite} metric=${metric} pairs=${mine.pairs} run=${run} other=${other} open=${open} />`}
      <div class="rc-aside">
        <${DirectionKey} counts=${counts} metric=${metric} on=${direction => open(selected, { direction })} />
        <p>${rate ? barsNote(suite, runs) : scatterNote(run, other, metric)}</p>
      </div>
    </div>
  </${Card}>`;
}

const Card = ({ title, answer, children }) => html`<section class="rc-card">
  <h2 class="rc-title">${title}</h2>
  ${answer ? html`<p class="rc-answer" title=${answer}>${answer}</p>` : null}
  ${children}
</section>`;

function Trend({ rows, groupRows, metric, axis, selected, vs, select }) {
  const series = trendSeries(rows, groupRows, axis, selected);
  const main = series.filter(s => s.main).flatMap(s => s.rows);
  const best = bestOf(main, metric);
  const hollow = main.find(d => d.run.id === vs?.id) ?? main.find(d => d.run.baseline);
  const ref = usePlot(width => clickable(trendPlot({ width, series, axis, metric, selected, hollow, best }), d => select(d.run.id)),
    [rows, groupRows, axis, metric, selected, vs]);
  const fmt = scoreFormat(metric);
  const sel = main.find(d => d.run.id === selected);
  const answer = !sel ? `The best run is ${best.run.label}, at ${fmt(best.score.value)}.`
    : sel === best ? `${sel.run.label} is the best run, at ${fmt(sel.score.value)}.`
    : sel.score.value === best.score.value ? `${sel.run.label} ties ${best.run.label} for the best score, ${fmt(sel.score.value)}.`
    : `${sel.run.label} scores ${fmt(sel.score.value)}; the best is ${best.run.label}, at ${fmt(best.score.value)}.`;
  return html`<${Card} title=${`${metric.label}, run by run`} answer=${answer}>
    <div class="rc-plot" ref=${ref}></div>
    <div class="rc-legend">
      ${series.length > 1 ? series.map(s => html`<span><${Swatch} style=${s.style} />${s.name}</span>`) : null}
      ${hollow ? html`<span><${Marker} hollow />${hollow.run.baseline ? "baseline" : "compared with"}</span>` : null}
      <span><${Marker} ring />${axis.series ? "best" : "best so far"}</span>
    </div>
  </${Card}>`;
}

function SpendChart({ rows, metric, spend, axis, selected, vs, select }) {
  const front = frontier(rows, metric);
  const money = spendFormat(spend, rows.map(r => r.spend?.value));
  const ref = usePlot(width => clickable(spendPlot({ width, rows, metric, spend, money, axis, selected, vs, front }), d => select(d.run.id)),
    [rows, metric, spend, axis, selected, vs]);
  return html`<${Card} title=${`${metric.label} vs ${spend.label}`} answer=${frontierAnswer(rows, metric, money, front, selected)}>
    <div class="rc-plot" ref=${ref}></div>
    <div class="rc-legend">
      <span><${Marker} fill="var(--text)" />on the frontier</span>
      <span><${Marker} fill="var(--tertiary)" opacity=${0.55} />costs more for less</span>
    </div>
  </${Card}>`;
}

function PairScatter({ suite, metric, pairs, run, other, open }) {
  const ref = usePlot(width => clickable(scatterPlot({ width, suite, metric, pairs, run, other }), d => open(run.id, { pair: d.key })),
    [suite, metric, pairs, run, other]);
  return html`<div class="rc-plot rc-square" ref=${ref}></div>`;
}

function PairBars({ bars, metric, selected, open }) {
  const ref = usePlot(width => clickable(barsPlot({ width, bars, metric, selected }), d => open(d.bar.run.id, { direction: DIRECTION[d.tone] })),
    [bars, metric, selected]);
  return html`<div class="rc-plot rc-grow" ref=${ref}></div>`;
}

function DirectionKey({ counts, metric, on }) {
  const m = moves(metric);
  return html`<div class="rc-key" role="group" aria-label="Open the items that moved">
    ${TONES.map(t => html`<button class=${cx("pf-dir", tone(t))} disabled=${!counts[t]} onClick=${() => on(DIRECTION[t])}>
      ${m[t].glyph} ${m[t].word} <strong>${counts[t]}</strong>
    </button>`)}
  </div>`;
}

const Swatch = ({ style }) => html`<svg class="rc-swatch rc-line" viewBox="0 0 16 8" aria-hidden="true">
  <line x1="1.5" y1="4" x2="14.5" y2="4" stroke=${style.color} stroke-opacity=${style.opacity} stroke-width="1.5" stroke-dasharray=${style.dash} stroke-linecap="round" />
</svg>`;

const Marker = ({ hollow, ring, fill = "var(--tertiary)", opacity = 1 }) => html`<svg class="rc-swatch" viewBox="0 0 16 16" aria-hidden="true">
  ${ring ? html`<circle cx="8" cy="8" r="6" fill="none" stroke="var(--text)" stroke-width="2" />` : null}
  <circle cx="8" cy="8" r="3.5" fill=${hollow ? "var(--surface)" : fill} fill-opacity=${opacity} stroke=${hollow ? "var(--tertiary)" : "none"} stroke-width="1.5" />
</svg>`;

// usePlot draws build(width) into the returned ref's element, again whenever the element's
// width or deps change.
function usePlot(build, deps) {
  const ref = useRef();
  const [width, setWidth] = useState(0);
  useEffect(() => {
    const observer = new ResizeObserver(([e]) => setWidth(Math.floor(e.contentRect.width)));
    observer.observe(ref.current);
    return () => observer.disconnect();
  }, []);
  useEffect(() => {
    if (!width) return;
    const plot = build(width);
    ref.current.replaceChildren(plot);
    return () => plot.remove();
  }, [width, ...deps]);
  return ref;
}

// clickable calls on(datum) when a click lands while a pointer mark points at a datum.
function clickable(plot, on) {
  plot.addEventListener("input", () => plot.classList.toggle("rc-pointing", plot.value != null));
  plot.addEventListener("click", () => plot.value != null && on(plot.value));
  return plot;
}

// trendAxis picks the trend's x: an ordered dimension swept with one line per value of
// another, else a numeric round, else a date, else run order.
function trendAxis(suite, runs) {
  const dims = suite.dimensions || [];
  const labelOf = (id, fallback) => dims.find(d => d.id === id)?.label ?? fallback;
  const varies = d => new Set(runs.map(r => r.dims[d.id])).size > 1;
  const along = dims.find(d => d.order && varies(d));
  const by = along && dims.find(d => d !== along && varies(d));
  if (by) {
    const domain = along.order.filter(v => runs.some(r => r.dims[along.id] === v));
    const x = r => r.dims[along.id];
    return { x, rank: r => domain.indexOf(x(r)), series: r => String(r.dims[by.id]), ticks: domain, scale: { type: "point", domain, label: along.label ?? along.id, tickFormat: String } };
  }
  if (runs.every(r => isNum(r.dims.round))) {
    const x = r => r.dims.round;
    return { x, rank: x, scale: { type: "linear", inset: 12, label: labelOf("round", "Round"), tickFormat: v => fmtNumber(v) } };
  }
  if (runs.every(r => ISO_DATE.test(r.dims.date ?? ""))) {
    const x = r => new Date(r.dims.date.slice(0, 10));
    const ticks = [...new Set(runs.map(r => r.dims.date.slice(0, 10)))].sort().map(d => new Date(d));
    return { x, rank: r => x(r).getTime(), ticks, scale: { type: "utc", inset: 12, label: labelOf("date", "Date"), tickFormat: fmtDate } };
  }
  const labels = new Map(runs.map(r => [r.id, r.label]));
  const domain = runs.map(r => r.id);
  return { x: r => r.id, rank: r => r.index, ticks: domain, scale: { type: "point", domain, label: "Run", tickFormat: id => labels.get(id) } };
}

// trendSeries is the trend's lines: one per value of the sweep's other dimension, else the
// rows, with one thin line per group beside them. Main lines carry dots and tips; the
// selected run's line carries its 95% band.
function trendSeries(rows, groupRows, axis, selected) {
  const sorted = rs => rs.filter(r => r.score && axis.x(r.run) !== undefined).sort((a, b) => axis.rank(a.run) - axis.rank(b.run));
  const scored = sorted(rows);
  if (axis.series) {
    const names = [...new Set(scored.map(r => axis.series(r.run)))];
    const chosen = scored.find(r => r.run.id === selected);
    const mine = name => chosen && axis.series(chosen.run) === name;
    const style = (name, i) => (names.length <= 3 ? SHADES[i] : mine(name) ? CHOSEN : FADED);
    return names.map((name, i) => ({ name, main: true, band: mine(name), style: style(name, i), rows: scored.filter(r => axis.series(r.run) === name) }));
  }
  if (!groupRows.length) return [{ name: "All items", main: true, band: true, style: PLAIN, rows: scored }];
  return [
    { name: "All items", main: true, band: true, style: SHADES[0], rows: scored },
    ...groupRows.map((g, i) => ({ name: g.label, main: false, style: SHADES[i + 1], rows: sorted(g.rows) })),
  ];
}

function trendPlot({ width, series, axis, metric, selected, hollow, best }) {
  const x = d => axis.x(d.run), y = d => d.score.value;
  const main = series.filter(s => s.main);
  const all = main.flatMap(s => s.rows);
  const values = series.flatMap(s => s.rows.flatMap(d => (s.band ? [d.score.lo, d.score.hi] : [d.score.value])));
  const plotWidth = width - MARGIN.marginLeft - MARGIN.marginRight;
  const xScale = { ...axis.scale, ...X_LABEL, tickSize: 4, ticks: axis.ticks ? thinTicks(axis.ticks, axis.scale.tickFormat, plotWidth) : 5 };
  return Plot.plot({
    width, height: HEIGHT, ...MARGIN, style: STYLE,
    x: xScale,
    y: scoreAxis(metric, values),
    marks: [
      Plot.frame({ anchor: "bottom", stroke: "var(--separator)" }),
      hollow ? Plot.ruleY([hollow.score.value], { stroke: "var(--tertiary)", strokeDasharray: "3 3" }) : null,
      ...series.filter(s => s.band).map(s => Plot.areaY(s.rows, { x, y1: d => d.score.lo, y2: d => d.score.hi, fill: s.style.color, fillOpacity: 0.14 })),
      ...series.map(s => Plot.line(s.rows, { x, y, stroke: s.style.color, strokeOpacity: s.style.opacity, strokeDasharray: s.style.dash, ...LINE })),
      ...main.map(s => Plot.dot(s.rows.filter(d => d !== hollow), { x, y, fill: s.style.color, fillOpacity: s.style.opacity, ...DOT })),
      hollow ? Plot.dot([hollow], { x, y, r: 3.5, fill: "var(--surface)", stroke: seriesOf(main, hollow).style.color, strokeWidth: 1.5 }) : null,
      Plot.dot([best], { x, y, r: 6.5, fill: "none", stroke: "var(--text)", strokeWidth: 2 }),
      Plot.dot(all.filter(d => d.run.id === selected), { x, y, fill: "var(--accent)", ...DOT }),
      Plot.tip(all, Plot.pointer({ x, y, title: scoreTip(metric), ...TIP })),
    ],
  });
}

function spendPlot({ width, rows, metric, spend, money, axis, selected, vs, front }) {
  const pts = rows.filter(r => r.score && r.spend);
  const x = d => d.spend.value, y = d => d.score.value;
  const xScale = spendAxis(pts.map(x), spend);
  const yScale = scoreAxis(metric, pts.map(y));
  const on = pts.filter(d => front.has(d.run.id)).sort((a, b) => x(a) - x(b));
  const off = pts.filter(d => !front.has(d.run.id));
  const base = pts.find(d => d.run.id === vs?.id);
  const sweep = axis.series ? pts.filter(d => axis.x(d.run) !== undefined).sort((a, b) => axis.rank(a.run) - axis.rank(b.run)) : [];
  const labels = placeLabels({ labeled: labelOrder(pts, front, selected), dots: pts, steps: on, rule: base && y(base), width, margin: MARGIN, xScale, yScale, x, y });
  return Plot.plot({
    width, height: HEIGHT, ...MARGIN, style: STYLE,
    x: xScale, y: yScale,
    marks: [
      Plot.frame({ anchor: "bottom", stroke: "var(--separator)" }),
      base ? Plot.ruleY([y(base)], { stroke: "var(--tertiary)", strokeDasharray: "3 3" }) : null,
      Plot.line(sweep, { x, y, z: d => axis.series(d.run), stroke: "var(--tertiary)", strokeOpacity: 0.35, strokeWidth: 1, strokeLinejoin: "round" }),
      Plot.line(on, { x, y, curve: "step-after", stroke: "var(--text)", strokeOpacity: 0.5, strokeWidth: 1 }),
      Plot.dot(off.filter(d => d !== base), { x, y, fill: "var(--tertiary)", fillOpacity: 0.55, ...DOT }),
      Plot.dot(on.filter(d => d !== base), { x, y, fill: "var(--text)", ...DOT }),
      base ? Plot.dot([base], { x, y, r: 3.5, fill: "var(--surface)", stroke: front.has(base.run.id) ? "var(--text)" : "var(--tertiary)", strokeWidth: 1.5 }) : null,
      Plot.dot(pts.filter(d => d.run.id === selected), { x, y, fill: "var(--accent)", ...DOT }),
      ...Object.entries(LABEL_SIDES).map(([side, o]) => Plot.text(labels.filter(l => l.side === side), { x, y, text: d => d.run.label, fill: "var(--secondary)", stroke: "var(--surface)", strokeWidth: 3, ...o })),
      Plot.dot(off, Plot.pointer({ x, y, fill: "var(--tertiary)", ...DOT })),
      Plot.tip(pts, Plot.pointer({ x, y, title: d => `${d.run.label}\n${scoreFormat(metric)(y(d))} for ${money(x(d))}`, ...TIP })),
    ],
  });
}

function scatterPlot({ width, suite, metric, pairs, run, other }) {
  const margin = { marginTop: 24, marginRight: 8, marginBottom: 36, marginLeft: 40 };
  const height = width - margin.marginLeft - margin.marginRight + margin.marginTop + margin.marginBottom;
  const [lo, hi] = metricRange(suite, metric, pairs.flatMap(d => [d.x, d.y]));
  const fmt = scoreFormat(metric);
  const scale = { domain: [lo, hi], ticks: niceTicks([lo, hi]), tickFormat: axisFormat(metric), labelArrow: "none" };
  const moved = d => (d.tone === "neutral" ? 0 : 1);
  const ordered = [...pairs].sort((a, b) => moved(a) - moved(b));
  return Plot.plot({
    width, height, ...margin, style: STYLE,
    x: { ...scale, ...X_LABEL, tickSize: 4, label: other.label },
    y: { ...scale, tickSize: 0, grid: true, label: run.label },
    marks: [
      Plot.frame({ anchor: "bottom", stroke: "var(--separator)" }),
      Plot.line([[lo, lo], [hi, hi]], { stroke: "var(--separator)", strokeWidth: 1 }),
      Plot.link(ordered, { x1: "x", y1: "x", x2: "x", y2: "y", stroke: d => TONE_FILL[d.tone], strokeWidth: 1.5, strokeOpacity: 0.4 }),
      Plot.dot(ordered, { x: "x", y: "y", fill: d => TONE_FILL[d.tone], ...DOT }),
      Plot.tip(ordered, Plot.pointer({ x: "x", y: "y", title: d => `${itemTitle(d)}\n${other.label} ${fmt(d.x)} → ${run.label} ${fmt(d.y)}`, ...TIP })),
    ],
  });
}

function barsPlot({ width, bars, metric, selected }) {
  const m = moves(metric);
  const ROW = 28, COUNT_W = 44;
  const max = Math.max(...bars.map(b => b.pairs.length));
  const marginLeft = Math.max(...bars.map(b => b.run.label.length)) * CHAR + 14;
  const segments = bars.flatMap(b => {
    let at = 0;
    return TONES.map(t => ({ bar: b, tone: t, count: b.counts[t], x1: at, x2: (at += b.counts[t]) })).filter(s => s.count);
  });
  const id = d => (d.bar ?? d).run.id;
  const count = (t, dx) => Plot.text(bars, {
    x: () => max, y: id, dx, text: b => `${m[t].glyph} ${b.counts[t]}`, textAnchor: "start",
    fill: b => (t === "neutral" || !b.counts[t] ? "var(--tertiary)" : `var(--${t}-text)`), fontWeight: t === "neutral" ? 500 : 600,
  });
  const label = (rows, fill, fontWeight) => Plot.text(rows, { x: 0, y: id, dx: -12, text: b => b.run.label, textAnchor: "end", fill, fontWeight });
  return Plot.plot({
    width, height: bars.length * ROW + 4, marginTop: 2, marginBottom: 2, marginLeft, marginRight: 3 * COUNT_W + 12, style: STYLE,
    x: { domain: [0, max], axis: null },
    y: { type: "band", domain: bars.map(id), axis: null, padding: 0 },
    marks: [
      ...TONES.map(t => Plot.barX(segments.filter(s => s.tone === t), {
        x1: "x1", x2: "x2", y: id, fill: TONE_FILL[t], fillOpacity: t === "neutral" ? 0.5 : 1, insetTop: 10, insetBottom: 10, insetLeft: 1, insetRight: 1, rx: 3,
      })),
      label(bars.filter(b => b.run.id !== selected), "var(--secondary)", 500),
      label(bars.filter(b => b.run.id === selected), "var(--text)", 600),
      count("positive", 16), count("neutral", 16 + COUNT_W), count("negative", 16 + 2 * COUNT_W),
      Plot.tip(segments, Plot.pointer({ x: d => (d.x1 + d.x2) / 2, y: id, title: d => `${d.bar.run.label} vs ${d.bar.vs.label}\n${d.count} ${m[d.tone].word}`, ...TIP })),
    ],
  });
}

// scoreAxis is the score's y scale: padded around the values, a rate kept within 0..1,
// lower-is-better running top-down.
function scoreAxis(metric, values) {
  let lo = Math.min(...values), hi = Math.max(...values);
  const pad = (hi - lo) * 0.08 || 0.05;
  lo -= pad;
  hi += pad;
  if (metric.kind === "rate") [lo, hi] = [Math.max(0, lo), Math.min(1, hi)];
  return { domain: [lo, hi], reverse: metric.better === "lower", ticks: niceTicks([lo, hi]), tickFormat: axisFormat(metric), tickSize: 0, grid: true, label: null };
}

// spendAxis is log when the spend spans more than 4x, so cheap runs do not pile up at zero.
function spendAxis(values, spend) {
  const lo = Math.min(...values), hi = Math.max(...values);
  const common = { tickFormat: spendTicks(spend.unit), tickSize: 4, label: spend.label, ...X_LABEL };
  if (lo > 0 && hi / lo > 4) {
    const domain = [lo / 1.35, hi * 1.35];
    return { type: "log", domain, ticks: logTicks(domain), ...common };
  }
  const pad = (hi - lo) * 0.1 || hi * 0.1 || 1;
  return { type: "linear", domain: [lo - pad, hi + pad], ticks: niceTicks([lo - pad, hi + pad]), ...common };
}

// niceTicks is the round values in [lo, hi] at the finest 1-2-5 step that gives at most five.
function niceTicks([lo, hi]) {
  for (let k = Math.floor(Math.log10(hi - lo || 1)) - 1; ; k++) {
    for (const m of [1, 2, 5]) {
      const step = m * 10 ** k, ticks = [];
      for (let v = Math.ceil(lo / step - 1e-9) * step; v <= hi + step * 1e-9; v += step) ticks.push(+v.toPrecision(12));
      if (ticks.length <= 5) return ticks;
    }
  }
}

// logTicks is the 1-2-5 steps within the domain, else 1-3, else powers of ten: at most five.
function logTicks([lo, hi]) {
  const steps = [[1, 2, 5], [1, 3], [1]].map(mantissas => {
    const ticks = [];
    for (let k = Math.floor(Math.log10(lo)); k <= Math.ceil(Math.log10(hi)); k++)
      for (const m of mantissas) if (m * 10 ** k >= lo && m * 10 ** k <= hi) ticks.push(+(m * 10 ** k).toPrecision(1));
    return ticks;
  });
  return steps.find(t => t.length <= 5) ?? steps.at(-1);
}

// The four places a label may sit beside its dot, as Plot text options, in order of
// preference: a frontier dot's own step leaves it to the right and arrives from below.
const LABEL_SIDES = {
  left: { textAnchor: "end", dx: -8 },
  above: { dy: -14 },
  right: { textAnchor: "start", dx: 8 },
  below: { dy: 14 },
};

const labelOrder = (pts, front, selected) => [
  ...pts.filter(d => d.run.id === selected),
  ...pts.filter(d => d.run.id !== selected && front.has(d.run.id)).sort((a, b) => a.spend.value - b.spend.value),
];

// placeLabels gives each labeled point in turn the first side whose label box stays inside
// the plot and clears every dot, every earlier label and, if it can, the frontier's steps
// and the baseline rule (the label's halo cuts a line it crosses); a point with no free
// side goes unlabeled.
function placeLabels({ labeled, dots, steps, rule, width, margin, xScale, yScale, x, y }) {
  const sx = Plot.scale({ x: { ...xScale, range: [margin.marginLeft, width - margin.marginRight] } });
  const sy = Plot.scale({ y: { ...yScale, range: [HEIGHT - margin.marginBottom, margin.marginTop] } });
  const at = d => [sx.apply(x(d)), sy.apply(y(d))];
  const taken = dots.map(d => { const [px, py] = at(d); return [px - 5, py - 5, px + 5, py + 5]; });
  const lines = steps.slice(1).flatMap((d, i) => {
    const [x0, y0] = at(steps[i]), [x1, y1] = at(d);
    return [[x0, y0 - 1, x1, y0 + 1], [x1 - 1, Math.min(y0, y1), x1 + 1, Math.max(y0, y1)]];
  });
  if (rule !== undefined) lines.push([margin.marginLeft, sy.apply(rule) - 1, width, sy.apply(rule) + 1]);
  const H = 12, GAP = 8;
  const clear = (box, others) => !others.some(([a0, b0, a1, b1]) => box[0] < a1 && a0 < box[2] && box[1] < b1 && b0 < box[3]);
  const inside = ([x0, y0, x1, y1]) => x0 >= margin.marginLeft && x1 <= width && y0 >= 0 && y1 <= HEIGHT - margin.marginBottom;
  const placed = [];
  for (const d of labeled) {
    const [px, py] = at(d), w = d.run.label.length * CHAR;
    const boxes = {
      right: [px + GAP, py - H / 2, px + GAP + w, py + H / 2],
      left: [px - GAP - w, py - H / 2, px - GAP, py + H / 2],
      above: [px - w / 2, py - GAP - H, px + w / 2, py - GAP],
      below: [px - w / 2, py + GAP, px + w / 2, py + GAP + H],
    };
    const sides = Object.keys(LABEL_SIDES).filter(s => inside(boxes[s]) && clear(boxes[s], taken));
    const side = sides.find(s => clear(boxes[s], lines)) ?? sides[0];
    if (!side) continue;
    taken.push(boxes[side]);
    placed.push({ ...d, side });
  }
  return placed;
}

// thinTicks keeps every k-th value of a point axis so its labels do not overlap.
function thinTicks(domain, format, plotWidth) {
  const widest = Math.max(...domain.map(v => String(format(v)).length)) * CHAR + 12;
  const every = Math.max(1, Math.ceil((domain.length * widest) / plotWidth));
  return domain.filter((_, i) => i % every === 0);
}

// pairTones is every item runId shares with vsId where both have a value: x is vsId's
// value, y runId's, tone whether the move is better, worse or within noise.
function pairTones(suite, state, metric, runs, runId, vsId, { judgeOnly, group }) {
  const runsById = new Map(runs.map(r => [r.id, r]));
  const value = item => itemValue(state, item, metric, runsById, { judgeOnly });
  return [...pairsOf(suite, runId, vsId)]
    .filter(([, p]) => !group || p.item.group === group)
    .map(([key, p]) => ({ key, item: p.item, x: value(p.vs), y: value(p.item) }))
    .filter(d => isNum(d.x) && isNum(d.y))
    .map(d => ({ ...d, tone: deltaTone(d.y - d.x, metric) }));
}

function tally(pairs) {
  const counts = { positive: 0, neutral: 0, negative: 0 };
  for (const p of pairs) counts[p.tone]++;
  return counts;
}

function bestOf(rows, metric) {
  const sign = metric.better === "lower" ? -1 : 1;
  return rows.reduce((a, b) => (sign * (b.score.value - a.score.value) > 0 ? b : a));
}

const seriesOf = (series, d) => series.find(s => s.rows.includes(d));

// metricRange is the metric's declared min..max, else the values' extent.
function metricRange(suite, metric, values) {
  const declared = metric.q ?? (suite.columns || []).find(c => c.id === metric.id);
  if (isNum(declared?.min) && isNum(declared?.max)) return [declared.min, declared.max];
  return [Math.min(...values), Math.max(...values)];
}

function frontierAnswer(rows, metric, money, front, selected) {
  const sel = rows.find(r => r.run.id === selected);
  if (!sel?.spend || !sel.score) return null;
  if (front.has(selected)) return `${sel.run.label} is on the frontier at ${money(sel.spend.value)}.`;
  const sign = metric.better === "lower" ? -1 : 1;
  const better = rows.filter(r => front.has(r.run.id) && r.spend.value <= sel.spend.value && sign * (r.score.value - sel.score.value) >= 0)
    .sort((a, b) => a.spend.value - b.spend.value)[0];
  if (!better) return `${sel.run.label} is off the frontier at ${money(sel.spend.value)}.`;
  return `Off the frontier: ${better.run.label} scores ${scoreFormat(metric)(better.score.value)} for ${money(better.spend.value)}.`;
}

function scatterNote(run, other, metric) {
  const side = metric.better === "lower" ? "Below" : "Above";
  return `Each dot is one item: its ${other.label} score across, its ${run.label} score up. ${side} the line it got better. Click a dot to open the item.`;
}

function barsNote(suite, runs) {
  const against = markedBaseline(suite, runs)?.label ?? "the run before it";
  return `Each bar is one run against ${against}, item by item. Click a part to open those items.`;
}

const itemTitle = d => d.item.short ?? d.item.title ?? d.key;

// scoreFormat and spendFormat print as the Runs view's table does; the tick formats drop
// trailing zeros and keep three significant digits for small spend.
const scoreFormat = metric => v => withUnit(fmtNumber(v, metric.decimals ?? 2), metric.unit);
const axisFormat = metric => v => withUnit(fmtNumber(v), metric.unit);

function spendFormat(spend, values) {
  const d = spend.decimals ?? autoDecimals(values);
  return v => withUnit(fmtNumber(v, d), spend.unit);
}

const spendTicks = unit => v => withUnit(Math.abs(v) >= 100 ? fmtNumber(v, 0) : v.toLocaleString("en-US", { maximumSignificantDigits: 3 }), unit);

function scoreTip(metric) {
  const fmt = scoreFormat(metric);
  return d => `${d.run.label}\n${fmt(d.score.value)}  (95% CI ${fmt(d.score.lo)}–${fmt(d.score.hi)})\n${d.n} items`;
}
