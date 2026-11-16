// Source code with line numbers, Prism highlighting and emphasized lines.
import { html, useMemo } from "../html.js";
import { cx } from "../ui/bits.js";

const ESCAPES = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" };
export const escapeHtml = s => s.replace(/[&<>"']/g, c => ESCAPES[c]);

// highlight returns Prism HTML for text, or escaped plain text when the language is unknown.
export function highlight(text, lang) {
  const id = lang?.toLowerCase();
  const grammar = id && window.Prism?.languages[id];
  return grammar ? window.Prism.highlight(text, grammar, id) : escapeHtml(text);
}

// splitLines splits highlighted HTML into one string per line. A token can span lines
// (block comments, template strings), so open spans are closed at each line end and
// reopened on the next line.
export function splitLines(src) {
  const lines = [];
  const open = [];
  let line = "";
  for (const part of src.split(/(<span[^>]*>|<\/span>|\n)/)) {
    if (part === "\n") {
      lines.push(line + "</span>".repeat(open.length));
      line = open.join("");
      continue;
    }
    if (part.startsWith("<span")) open.push(part);
    else if (part === "</span>") open.pop();
    line += part;
  }
  lines.push(line);
  return lines;
}

export const normalize = text => (text ?? "").replace(/\r\n?/g, "\n").replace(/\n$/, "");

// CodeLines is one numbered, highlighted code panel; wrap folds long lines instead of
// scrolling sideways. The code and compare renderers share it.
export function CodeLines({ code, language, start = 1, highlight: marks, wrap, maxLines }) {
  const lines = useMemo(() => splitLines(highlight(normalize(code), language)), [code, language]);
  const marked = new Set(marks ?? []);
  return html`<div class=${cx("code-panel", wrap && "wrap")} style=${capStyle(maxLines)}><table class="code-table"><tbody>
    ${lines.map((line, i) => {
      const n = start + i;
      return html`<tr key=${n} class=${cx(marked.has(n) && "code-hl")} data-line=${n}>
        <td class="code-num">${n}</td>
        <td class="code-text" dangerouslySetInnerHTML=${{ __html: line }}></td>
      </tr>`;
    })}
  </tbody></table></div>`;
}

// capStyle sets how many lines a code panel shows before it scrolls inside itself.
export const capStyle = maxLines => (maxLines ? `--max-lines: ${maxLines}` : undefined);

export default function CodeArtifact({ a }) {
  return html`<${CodeLines} code=${a.code} language=${a.language} start=${a.start} highlight=${a.highlight} maxLines=${a.max_lines} />`;
}

CodeArtifact.head = ({ a }) => (a.language ? html`<span class="muted">${a.language}</span>` : null);
