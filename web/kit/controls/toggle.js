// Yes/no. On its own row it is a switch (the row label names it); inline beside
// another question or on a list row it is a pill that carries its own label.
// Per-row pills omit the key cap: the key acts on the cursor row and the help sheet lists it.
import { html } from "../html.js";
import { cx, Key } from "../ui/bits.js";

export default function Toggle({ q, value, proposed, set, row, inline }) {
  const on = value === true;
  // Turning off a proposed "yes" stores false, so the proposal does not come back.
  const flip = () => set(on ? (proposed?.value === true ? false : undefined) : true);
  if (row === undefined && !inline) {
    return html`<button class=${cx("switch", on && "on")} role="switch" aria-checked=${on} aria-label=${q.label} onClick=${flip}>
      <span class="switch-track"></span>
    </button>`;
  }
  return html`<button class=${cx("chip", row !== undefined && "small", on && "on")} aria-pressed=${on} onClick=${flip}>
    ${on ? `✓ ${q.label}` : q.label}${q.key && row === undefined ? html`<${Key} k=${q.key} />` : null}
  </button>`;
}

Toggle.rowPlacement = "lead";
Toggle.labelKey = true;
