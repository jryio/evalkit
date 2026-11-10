// Single choice as a segmented control. Options show their icon, else a tone dot.
// The primary question shows number keys; clicking the selected option clears it.
import { html } from "../html.js";
import { cx, tone, Key, Dot, Icon } from "../ui/bits.js";
import { optionOf } from "../core/derive.js";

// compact (set by dense renderers such as tables) shows icons only when every option has one.
// A per-row value that is only the agent's proposal renders dashed until the human confirms it.
export default function Choice({ q, value, answer, set, compact, row }) {
  const iconsOnly = compact && q.options.every(o => o.icon);
  const tentative = row !== undefined && answer === undefined;
  return html`<div class=${cx("segmented", iconsOnly && "icons-only")} role="radiogroup" aria-label=${q.label}>
    ${q.options.map((o, i) => {
      const on = value === o.value;
      return html`<button class=${cx("seg", on && "on", on && tentative && "tentative", o.icon && "has-icon", tone(o.tone))} role="radio" aria-checked=${on}
          title=${o.label} aria-label=${o.label} onClick=${() => set(answer === o.value ? undefined : o.value)}>
        ${o.icon ? html`<${Icon} name=${o.icon} />` : o.tone ? html`<${Dot} tone=${o.tone} />` : null}
        ${iconsOnly ? null : o.label}
        ${q.primary ? html`<${Key} k=${String(i + 1)} />` : o.key ? html`<${Key} k=${o.key} />` : null}
      </button>`;
    })}
  </div>`;
}

// note: the agent's reason while its proposal stands, else what the human changed.
Choice.note = ({ q, value, proposed }) => {
  if (!proposed) return null;
  if (value === proposed.value) return proposed.why ?? null;
  return `Changed from ${optionOf(q, proposed.value)?.label ?? proposed.value}`;
};

Choice.labelKey = true;
