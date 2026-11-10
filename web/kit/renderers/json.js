// A collapsible JSON tree. Objects and arrays fold with a caret; a.open is how many
// levels start expanded.
import { html, useState } from "../html.js";

export default function JsonArtifact({ a }) {
  return html`<div class="json"><${Node} value=${a.data} depth=${0} open=${a.open ?? 1} /></div>`;
}

const plural = (n, word) => `${n} ${word}${n === 1 ? "" : "s"}`;
const P = ({ children }) => html`<span class="json-p">${children}</span>`;
const Line = ({ lead, children }) => html`<div class="json-line">${lead ?? html`<span class="json-gutter"></span>`}<span class="json-text">${children}</span></div>`;

function Node({ name, value, depth, open, comma }) {
  const [expanded, setExpanded] = useState(depth < open);
  const entries = value !== null && typeof value === "object" ? Object.entries(value) : [];
  const key = name !== undefined ? html`<span class="json-key">${JSON.stringify(name)}</span><${P}>: </${P}>` : null;
  const tail = comma ? html`<${P}>,</${P}>` : null;

  if (!entries.length) return html`<${Line}>${key}<${Leaf} value=${value} />${tail}</${Line}>`;

  const isArray = Array.isArray(value);
  const [opener, closer] = isArray ? ["[", "]"] : ["{", "}"];
  const caret = html`<button class="json-caret" aria-expanded=${expanded} aria-label=${expanded ? "Collapse" : "Expand"}
    onClick=${() => setExpanded(!expanded)}>${expanded ? "▾" : "▸"}</button>`;

  if (!expanded) {
    const count = plural(entries.length, isArray ? "item" : "key");
    return html`<${Line} lead=${caret}>${key}<${P}>${opener}…${closer}</${P}><span class="json-count">${count}</span>${tail}</${Line}>`;
  }
  return html`<div>
    <${Line} lead=${caret}>${key}<${P}>${opener}</${P}></${Line}>
    <div class="json-children">${entries.map(([k, v], i) => html`<${Node} key=${k} name=${isArray ? undefined : k}
      value=${v} depth=${depth + 1} open=${open} comma=${i < entries.length - 1} />`)}</div>
    <${Line}><${P}>${closer}</${P}>${tail}</${Line}>
  </div>`;
}

function Leaf({ value }) {
  if (Array.isArray(value)) return html`<${P}>[]</${P}>`;
  if (value !== null && typeof value === "object") return html`<${P}>{}</${P}>`;
  const kind = typeof value === "string" ? "string" : typeof value === "number" ? "number" : "literal";
  return html`<span class=${`json-${kind}`}>${JSON.stringify(value)}</span>`;
}
