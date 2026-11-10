// Source list: suite title, progress, then items under their group headers.
import { html, useEffect, useRef } from "../html.js";
import { cx, Dot } from "./bits.js";
import { progress, statusTone, trailing } from "../core/derive.js";

export function Sidebar({ suite, items, state, current, go, openHelp }) {
  const { reviewed, total } = progress(suite, state);
  const sections = suite.groups?.length
    ? suite.groups.map(g => ({ label: g.label, items: items.filter(i => i.group === g.id) })).filter(s => s.items.length)
    : [{ label: "", items }];
  return html`<aside class="sidebar">
    <div class="sb-head">
      <div class="sb-title">${suite.title}</div>
      ${suite.description ? html`<div class="sb-desc" title=${suite.description}>${suite.description}</div>` : null}
      <div class="progress"><span style=${{ width: `${total ? (100 * reviewed) / total : 0}%` }}></span></div>
      <div class="sb-count">${reviewed} of ${total} reviewed</div>
    </div>
    <nav class="sb-nav">
      ${sections.map(s => html`
        ${s.label ? html`<div class="sb-section">${s.label}</div>` : null}
        ${s.items.map(item => html`<${NavRow} key=${item.id} item=${item} on=${item === items[current]}
          tone=${statusTone(suite, state, item)} trail=${trailing(suite, state, item)} onClick=${() => go(items.indexOf(item))} />`)}
      `)}
    </nav>
    <button class="sb-foot" onClick=${openHelp}><kbd>?</kbd> Keyboard shortcuts</button>
  </aside>`;
}

function NavRow({ item, on, tone, trail, onClick }) {
  const ref = useRef();
  useEffect(() => {
    if (on) ref.current.scrollIntoView({ block: "nearest" });
  }, [on]);
  return html`<button ref=${ref} class=${cx("nav-row", on && "on")} onClick=${onClick} aria-current=${on ? "true" : undefined}>
    <${Dot} tone=${tone} />
    <span class="nav-text">
      <span class="nav-title">${item.short ?? item.title}</span>
      ${item.subtitle ? html`<span class="nav-sub">${item.subtitle}</span>` : null}
    </span>
    <${Trail} trail=${trail} />
  </button>`;
}

function Trail({ trail }) {
  if (!trail) return html`<span></span>`;
  if (trail.kind === "reviewed") return html`<span class="trail-check" aria-label="Reviewed">✓</span>`;
  return html`<span class=${cx("trail-pill", trail.kind === "done" && "done")}>${trail.label}</span>`;
}
