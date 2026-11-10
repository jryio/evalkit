// Rows of title, detail and links. Per-row controls: `lead` in a leading slot (or the
// row's badge when it has none), `below` under the row text.
import { html } from "../html.js";
import { cx, tone } from "../ui/bits.js";

export default function List({ a, rowControls, cursor }) {
  const controls = a.rows.map(r => rowControls(r));
  const hasLead = controls.some(c => c.lead) || a.rows.some(r => r.badge);
  return html`<div class=${cx("list", !hasLead && "no-lead")}>
    ${a.rows.map((r, i) => html`<div class=${cx("list-row", cursor === r.id && "row-cursor")} key=${r.id} data-row=${`${a.id}:${r.id}`}>
      ${hasLead ? html`<div class="list-lead">${controls[i].lead || (r.badge ? html`<span class=${cx("row-badge", tone(r.badge.tone))}>${r.badge.label}</span>` : null)}</div>` : null}
      <div>
        <div class="row-title">${r.title}${r.at ? html` <${At} at=${r.at} />` : null}</div>
        ${r.detail ? html`<div class="row-detail">${r.detail}</div>` : null}
        ${r.links?.length ? html`<div class="links">${r.links.map(l => l.url ? html`<a href=${l.url} target="_blank" rel="noreferrer">${l.label}</a>` : html`<span>${l.label}</span>`)}</div>` : null}
        ${controls[i].below ? html`<div class="row-controls">${controls[i].below}</div>` : null}
      </div>
    </div>`)}
  </div>`;
}

// At links a row to a line in another artifact and flashes that line.
function At({ at }) {
  const jump = e => {
    e.preventDefault();
    const el = document.querySelector(`[data-artifact="${at.artifact}"] [data-line="${at.line}"]`);
    if (!el) return;
    el.scrollIntoView({ block: "center", behavior: "smooth" });
    el.classList.remove("flash");
    void el.offsetWidth; // restart the animation
    el.classList.add("flash");
  };
  return html`<a class="row-at" href="#" onClick=${jump}>${at.label ?? `line ${at.line}`}</a>`;
}
