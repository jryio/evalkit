// An agent transcript: one row per step, the step and role on the left, the message or
// tool call on the right. Per-row controls: `lead` under the role, `below` under the content.
import { html, useState } from "../html.js";
import { cx } from "../ui/bits.js";

const ROLES = { system: "System", user: "User", assistant: "Assistant", tool_call: "Tool call", tool_result: "Tool result" };
const pretty = v => (typeof v === "string" ? v : JSON.stringify(v, null, 2));

export default function TranscriptArtifact({ a, rowControls, cursor }) {
  return html`<div class="transcript">${a.rows.map((r, i) => {
    const { lead, below } = rowControls(r);
    return html`<div class=${cx("tr-row", cursor === r.id && "row-cursor")} key=${r.id} data-row=${`${a.id}:${r.id}`}>
      <div>
        <div class="tr-step">Step ${i + 1}</div>
        <div class="tr-role">${ROLES[r.role] ?? r.role}</div>
        ${lead ? html`<div class="tr-lead">${lead}</div>` : null}
      </div>
      <div class="tr-content">
        ${r.error ? html`<div class="tr-error-tag">Error</div>` : null}
        <${Content} r=${r} />
        ${below ? html`<div class="row-controls">${below}</div>` : null}
      </div>
    </div>`;
  })}</div>`;
}

const CLAMP = 12; // lines of a tool block shown before "Show all"

function Content({ r }) {
  const block = cx("tr-block", r.error && "tr-error");
  switch (r.role) {
    case "tool_call":
      return html`<${Clamp} text=${pretty(r.input ?? {})}>${body => html`<pre class=${block}><span class="tr-name">${r.name}</span><span class="tr-p">(</span>${body}<span class="tr-p">)</span></pre>`}</${Clamp}>`;
    case "tool_result":
      return html`<${Clamp} text=${pretty(r.output ?? "")}>${body => html`<pre class=${block}>${body}</pre>`}</${Clamp}>`;
    default:
      return html`<div class=${cx(`tr-${r.role}`, r.error && "tr-error")}>${r.text}</div>`;
  }
}

// Clamp shows the first CLAMP lines of a long tool block, with a button for the rest.
function Clamp({ text, children }) {
  const [open, setOpen] = useState(false);
  const lines = text.split("\n");
  if (open || lines.length <= CLAMP + 2) return children(text);
  return html`${children(lines.slice(0, CLAMP).join("\n") + "\n…")}
    <button class="tr-more" onClick=${() => setOpen(true)}>Show all ${lines.length} lines</button>`;
}
