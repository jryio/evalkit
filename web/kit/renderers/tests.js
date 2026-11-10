// Test results as a tree. Groups take the worst status below them and open when
// they contain a failure; a failed leaf shows its output.
import { html } from "../html.js";
import { cx } from "../ui/bits.js";

const GLYPHS = { pass: "✓", fail: "✕", skip: "–" };

const isGroup = n => n.children?.length > 0;
const leaves = n => (isGroup(n) ? n.children.flatMap(leaves) : [n]);
const leafStatus = n => n.status ?? "skip";

function tally(nodes) {
  const statuses = nodes.flatMap(leaves).map(leafStatus);
  const count = s => statuses.filter(x => x === s).length;
  return { pass: count("pass"), fail: count("fail"), skip: count("skip") };
}

const groupStatus = t => (t.fail ? "fail" : t.pass ? "pass" : "skip");
const sum = xs => (xs.some(x => x !== undefined) ? xs.reduce((s, x) => s + (x ?? 0), 0) : undefined);
const duration = n => n.duration ?? (isGroup(n) ? sum(n.children.map(duration)) : undefined);
const fmtDuration = s => (s < 1 ? `${Math.round(s * 1000)} ms` : `${s.toFixed(1)} s`);
const countText = t => [t.pass && `${t.pass} passed`, t.fail && `${t.fail} failed`, t.skip && `${t.skip} skipped`].filter(Boolean).join(" · ");

export default function TestsArtifact({ a }) {
  return html`<div class="tests"><div class="tests-tree">
    ${a.tests.map((n, i) => html`<${TestNode} key=${i} n=${n} depth=${0} />`)}
  </div></div>`;
}

function TestNode({ n, depth }) {
  const style = { "--depth": depth };
  if (!isGroup(n)) {
    const status = leafStatus(n);
    return html`<div class="t-leaf" style=${style}>
      <div class="t-row"><span></span><${Row} n=${n} status=${status} /></div>
      ${status === "fail" && n.output ? html`<pre class="t-output">${n.output}</pre>` : null}
    </div>`;
  }
  const counts = tally([n]);
  const status = groupStatus(counts);
  return html`<details class="t-group" open=${status === "fail"} style=${style}>
    <summary class="t-row"><${Row} n=${n} status=${status} counts=${counts} /></summary>
    ${n.children.map((c, i) => html`<${TestNode} key=${i} n=${c} depth=${depth + 1} />`)}
  </details>`;
}

function Row({ n, status, counts }) {
  const d = duration(n);
  return html`
    <span class=${cx("t-glyph", `t-${status}`)} aria-label=${status}>${GLYPHS[status]}</span>
    <span class="t-name">
      <span class=${counts ? "t-group-name" : "t-leaf-name"}>${n.name}</span>
      ${counts ? html`<span class="t-counts">${countText(counts)}</span>` : null}
    </span>
    <span class="t-duration">${d > 0 ? fmtDuration(d) : ""}</span>`;
}

// head summarizes leaf counts and the total duration.
TestsArtifact.head = ({ a }) => {
  const t = tally(a.tests);
  const total = sum(a.tests.map(duration));
  return html`<span class="muted tests-summary">
    ${t.pass} passed · <span class=${cx(t.fail > 0 && "tests-failed")}>${t.fail} failed</span> · ${t.skip} skipped${total !== undefined ? ` · ${total.toFixed(2)} s` : ""}
  </span>`;
};
