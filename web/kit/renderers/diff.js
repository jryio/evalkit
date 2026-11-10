// Unified diff: one header row per file, hunk headers, old and new line numbers.
import { html, useMemo } from "../html.js";
import { cx } from "../ui/bits.js";
import { highlight, normalize } from "./code.js";

const HUNK = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/;
const SIGNS = { ctx: "", add: "+", del: "−" };
const cleanPath = p => p.split("\t")[0].replace(/^[ab]\//, "");

// parseDiff returns [{ path, rows }]. Rows: { kind: "hunk" | "note", text } or
// { kind: "ctx" | "add" | "del", old?, new?, text }.
export function parseDiff(text) {
  const lines = normalize(text).split("\n");
  const files = [];
  let file = null;
  let inHunk = false;
  let oldNo = 0, newNo = 0, oldLeft = 0, newLeft = 0;
  const startFile = () => {
    file = { path: "", rows: [] };
    files.push(file);
    inHunk = false;
  };

  lines.forEach((line, i) => {
    if (line.startsWith("diff ")) {
      startFile();
      file.path = line.match(/ b\/(.+)$/)?.[1] ?? "";
      return;
    }
    const hunk = line.match(HUNK);
    if (hunk) {
      if (!file) startFile();
      [oldNo, oldLeft, newNo, newLeft] = [+hunk[1], +(hunk[2] ?? 1), +hunk[3], +(hunk[4] ?? 1)];
      inHunk = true;
      file.rows.push({ kind: "hunk", text: line });
      return;
    }
    if (inHunk) {
      // Once the hunk's counts are used up, a ---/+++ pair starts the next file of a plain
      // unified diff. Before that, the same pair is a deleted and an added line.
      const counted = oldLeft > 0 || newLeft > 0;
      const nextFile = !counted && line.startsWith("--- ") && lines[i + 1]?.startsWith("+++ ");
      const sign = line[0];
      if (!nextFile && sign === "+") {
        file.rows.push({ kind: "add", new: newNo++, text: line.slice(1) });
        newLeft--;
        return;
      }
      if (!nextFile && sign === "-") {
        file.rows.push({ kind: "del", old: oldNo++, text: line.slice(1) });
        oldLeft--;
        return;
      }
      if (sign === " " || (line === "" && counted)) {
        file.rows.push({ kind: "ctx", old: oldNo++, new: newNo++, text: line.slice(1) });
        oldLeft--;
        newLeft--;
        return;
      }
      if (sign === "\\") {
        file.rows.push({ kind: "note", text: line });
        return;
      }
      if (line === "") return;
      inHunk = false;
    }
    if (line.startsWith("--- ")) {
      if (!file || file.rows.length) startFile();
      const path = line.slice(4);
      if (!file.path && path !== "/dev/null") file.path = cleanPath(path);
    } else if (line.startsWith("+++ ")) {
      const path = line.slice(4);
      if (file && path !== "/dev/null") file.path = cleanPath(path);
    } else if (line.startsWith("rename to ") && file) {
      file.path = line.slice(10);
    }
  });
  return files;
}

export default function DiffArtifact({ a }) {
  const files = useMemo(() => parseDiff(a.diff), [a.diff]);
  const marked = new Set(a.highlight ?? []);
  const rowId = n => `diff-${a.id ?? "d"}-${n}`;
  return html`<div class="code-panel"><table class="code-table diff-table">
    ${files.map((f, i) => html`<tbody key=${i}>
      ${f.path ? html`<tr class="diff-file"><td colspan="4">${f.path}</td></tr>` : null}
      ${f.rows.map((r, j) => html`<${DiffRow} key=${j} r=${r} lang=${a.language}
        hl=${r.new !== undefined && marked.has(r.new)} rowId=${rowId(r.new)} />`)}
    </tbody>`)}
  </table></div>`;
}

function DiffRow({ r, lang, hl, rowId }) {
  if (r.kind === "hunk" || r.kind === "note") {
    return html`<tr class=${`diff-${r.kind}`}><td class="code-num"></td><td class="code-num"></td><td class="code-sign"></td><td class="code-text" title=${r.text}>${r.text}</td></tr>`;
  }
  return html`<tr class=${cx(`diff-${r.kind}`, hl && "diff-hl")} data-line=${r.new} id=${hl ? rowId : undefined}>
    <td class="code-num">${r.old}</td>
    <td class="code-num">${r.new}</td>
    <td class="code-sign">${SIGNS[r.kind]}</td>
    <td class="code-text" dangerouslySetInnerHTML=${{ __html: highlight(r.text, lang) }}></td>
  </tr>`;
}

DiffArtifact.head = ({ a }) => {
  const rows = parseDiff(a.diff).flatMap(f => f.rows);
  const count = kind => rows.filter(r => r.kind === kind).length;
  return html`<span class="diff-counts"><span class="diff-count-add">+${count("add")}</span><span class="diff-count-del">−${count("del")}</span></span>`;
};
