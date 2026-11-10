// An outward-facing action gate. The human approves; the agent acts after the send
// and records the result (`evalkit done`), which turns the button into a link.
import { html } from "../html.js";
import { cx, Key, fmtDate } from "../ui/bits.js";

export default function Action({ q, item, value, set }) {
  const done = item.done?.[q.id];
  if (done) {
    const text = `✓ ${q.states.done?.button ?? "Done"}`;
    return done.url ? html`<a class="done-link" href=${done.url} target="_blank" rel="noreferrer">${text} ↗</a>` : html`<span class="done-link">${text}</span>`;
  }
  const on = value === true;
  return html`<button class=${cx("approve", on && "on")} aria-pressed=${on} onClick=${() => set(on ? undefined : true)}>
    ${on ? `✓ ${q.states.on?.button ?? "Approved"}` : q.states.off.button}${q.key ? html`<${Key} k=${q.key} />` : null}
  </button>`;
}

Action.note = ({ q, item, value }) => {
  const done = item.done?.[q.id];
  if (done) return [done.at && fmtDate(done.at), done.note, q.states.done?.note].filter(Boolean).join(" · ") || null;
  return (value === true ? q.states.on?.note : q.states.off.note) ?? null;
};
