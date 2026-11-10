// Line or grouped-bar chart drawn with uPlot. Colors are read from the kit tokens,
// so the chart is rebuilt when the color scheme flips.
import { html, useEffect, useRef } from "../html.js";
import { cx, withUnit } from "../ui/bits.js";
import uPlot from "uplot";

const HEIGHT = 240;
const TONES = ["accent", "neutral", "warning", "positive", "negative"];
const BAR_MAX = 24;
const BAR_GAP = 2;
const BAR_RADIUS = 4;
const ISO = /^\d{4}-\d{2}-\d{2}(?:[T ]|$)/;

const decimalsOf = v => (String(+v.toFixed(6)).split(".")[1] ?? "").length;
const fmtNum = (v, d) => v.toLocaleString("en-US", { minimumFractionDigits: d, maximumFractionDigits: d });

export default function ChartArtifact({ a }) {
  const ref = useRef();
  useEffect(() => {
    const el = ref.current;
    let plot, width;
    const build = () => {
      plot?.destroy();
      width = el.clientWidth;
      const ax = xAxis(a);
      plot = new uPlot(options(a, ax, width), [ax.values, ...a.series.map(s => s.values)], el);
    };
    build();
    const resize = new ResizeObserver(() => {
      if (el.clientWidth === width) return;
      width = el.clientWidth;
      plot.setSize({ width, height: HEIGHT });
    });
    resize.observe(el);
    const scheme = matchMedia("(prefers-color-scheme: dark)");
    scheme.addEventListener("change", build);
    return () => {
      resize.disconnect();
      scheme.removeEventListener("change", build);
      plot.destroy();
    };
  }, [a]);
  return html`<div class=${cx("chart", a.type === "bar" ? "chart-bar" : "chart-line")}><div ref=${ref}></div></div>`;
}

// xAxis maps a.x to plot positions: unix seconds for ISO times, numbers as given,
// or indices for category labels. pos converts a mark's x the same way.
function xAxis(a) {
  const x = a.x ?? a.series[0].values.map((_, i) => i);
  if (x.every(v => typeof v === "number")) return { kind: "number", values: x, pos: Number };
  if (x.every(v => typeof v === "string" && ISO.test(v) && !Number.isNaN(Date.parse(v)))) {
    const pos = v => (typeof v === "number" ? v : Date.parse(v) / 1000);
    return { kind: "time", values: x.map(pos), pos };
  }
  const labels = x.map(String);
  return { kind: "category", values: labels.map((_, i) => i), labels, pos: v => (typeof v === "number" ? v : labels.indexOf(String(v))) };
}

function tokens() {
  const css = getComputedStyle(document.documentElement);
  const token = name => css.getPropertyValue(`--${name}`).trim();
  return { token, family: token("font") };
}

function options(a, ax, width) {
  const { token, family } = tokens();
  const bar = a.type === "bar";
  const colors = a.series.map((s, i) => token(s.tone ?? TONES[i % TONES.length]));
  const fmtY = v => withUnit(v.toLocaleString("en-US", { maximumFractionDigits: 3 }), a.unit);
  const axis = { stroke: token("secondary"), font: `12px ${family}`, ticks: { show: false }, gap: 6 };
  const slot = minStep(ax.values);
  const n = a.series.length;

  return {
    width,
    height: HEIGHT,
    padding: [a.marks?.length ? 28 : 12, 24, 0, 0],
    scales: {
      x: { time: ax.kind === "time", range: bar ? (u, lo, hi) => [lo - slot / 2, hi + slot / 2] : undefined },
      y: { range: (u, lo, hi) => yRange(lo, hi, a.y_min ?? (bar ? Math.min(0, lo) : undefined)) },
    },
    axes: [
      {
        ...axis,
        size: ax.kind === "time" ? 44 : 28,
        space: 80,
        grid: { show: false },
        border: { show: true, stroke: token("separator"), width: 1 },
        ...(ax.kind === "category" && { splits: () => ax.values, values: (u, splits) => splits.map(i => ax.labels[i] ?? "") }),
      },
      {
        ...axis,
        grid: { stroke: token("separator"), width: 1 },
        values: (u, splits) => {
          const d = Math.max(0, ...splits.map(decimalsOf));
          return splits.map(v => withUnit(fmtNum(v, d), a.unit));
        },
        size: labelWidth,
      },
    ],
    series: [
      {},
      ...a.series.map((s, i) => ({
        label: s.label,
        stroke: colors[i],
        width: bar ? 0 : 2,
        fill: bar ? colors[i] : undefined,
        paths: bar ? groupedBars(i, n, slot) : undefined,
        points: { show: false },
        value: (u, v) => (v == null ? "" : fmtY(v)),
      })),
    ],
    cursor: {
      x: !bar,
      y: false,
      drag: { x: false, y: false, setScale: false },
      points: bar ? { show: false } : { size: 10, width: 2, fill: (u, i) => colors[i - 1], stroke: () => token("surface") },
    },
    legend: { markers: { width: 0, fill: (u, i) => colors[i - 1] } },
    hooks: { draw: [u => drawMarks(u, a.marks ?? [], ax, token, family)] },
  };
}

function yRange(lo, hi, min) {
  if (lo == null || hi == null) return [min ?? 0, 1];
  const [niceLo, niceHi] = uPlot.rangeNum(min ?? lo, hi, 0.1, true);
  return [min ?? niceLo, niceHi];
}

const minStep = xs => {
  const sorted = [...new Set(xs)].sort((p, q) => p - q);
  const steps = sorted.slice(1).map((x, i) => x - sorted[i]);
  return steps.length ? Math.min(...steps) : 1;
};

// labelWidth sizes the y axis to its longest tick label so labels with units never clip.
function labelWidth(u, values, axisIdx) {
  if (!values?.length) return 40;
  const axis = u.axes[axisIdx];
  u.ctx.font = axis.font[0];
  const widest = Math.max(...values.map(v => u.ctx.measureText(v).width)) / devicePixelRatio;
  return Math.ceil(widest + axis.gap + 6);
}

// groupedBars draws series i of n beside its siblings: each group fills 70% of the
// slot between x values, bars cap at 24px with a 2px gap and 4px rounded tops.
function groupedBars(i, n, slot) {
  const geometry = u => {
    const pxPerUnit = u.bbox.width / devicePixelRatio / (u.scales.x.max - u.scales.x.min);
    const bar = Math.max(2, Math.min(BAR_MAX, (slot * pxPerUnit * 0.7 - BAR_GAP * (n - 1)) / n));
    const left = -(n * bar + (n - 1) * BAR_GAP) / 2 + i * (bar + BAR_GAP);
    return { size: bar / pxPerUnit, left: left / pxPerUnit, radius: Math.min(0.5, BAR_RADIUS / bar) };
  };
  return uPlot.paths.bars({
    radius: u => [geometry(u).radius, 0],
    disp: {
      x0: { unit: 1, values: u => u.data[0].map(x => x + geometry(u).left) },
      size: { unit: 1, values: u => u.data[0].map(() => geometry(u).size) },
    },
  });
}

function drawMarks(u, marks, ax, token, family) {
  const { ctx, bbox } = u;
  const pr = devicePixelRatio;
  ctx.save();
  ctx.font = `${12 * pr}px ${family}`;
  ctx.textAlign = "left";
  ctx.textBaseline = "bottom";
  ctx.lineWidth = pr;
  ctx.setLineDash([4 * pr, 4 * pr]);
  for (const m of marks) {
    const v = ax.pos(m.x);
    if (!Number.isFinite(v) || v < u.scales.x.min || v > u.scales.x.max) continue;
    const x = Math.round(u.valToPos(v, "x", true)) + (pr % 2) / 2;
    ctx.strokeStyle = token("tertiary");
    ctx.beginPath();
    ctx.moveTo(x, bbox.top);
    ctx.lineTo(x, bbox.top + bbox.height);
    ctx.stroke();
    const w = ctx.measureText(m.label).width;
    ctx.fillStyle = token("secondary");
    ctx.fillText(m.label, Math.min(Math.max(x - w / 2, 0), ctx.canvas.width - w), bbox.top - 6 * pr);
  }
  ctx.restore();
}
