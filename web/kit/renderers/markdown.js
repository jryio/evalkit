// Markdown. Raw HTML in the source shows as text, never markup; links open in a new tab.
import { marked } from "marked";
import { html, useMemo } from "../html.js";
import { cx } from "../ui/bits.js";

const ENTITIES = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" };
const escape = s => s.replace(/[&<>"']/g, c => ENTITIES[c]);

marked.use({
  renderer: {
    html: ({ text, block }) => (block ? `<p>${escape(text)}</p>` : escape(text)),
    link({ href, tokens }) {
      const text = this.parser.parseInline(tokens);
      if (/^\s*javascript:/i.test(href)) return text;
      return `<a href="${escape(href)}" target="_blank" rel="noreferrer">${text}</a>`;
    },
  },
});

export default function MarkdownArtifact({ a, inContext }) {
  const body = useMemo(() => marked.parse(a.text), [a.text]);
  return html`<div class=${cx("md", !inContext && "md-panel")} dangerouslySetInnerHTML=${{ __html: body }}></div>`;
}
