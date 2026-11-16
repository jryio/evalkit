// PierreDiff draws a split or unified diff with @pierre/diffs (web/vendor/diffs.esm.js,
// loaded on first use). It takes two files (oldFile, newFile) or a patch. The Pierre light
// and dark themes follow the system setting, like the rest of the page.
import { html, useEffect, useRef, useState } from "../html.js";

let lib;
const load = () => (lib ??= import("/vendor/diffs.esm.js"));

// The bundle carries one grammar per family; map the ids the code renderer accepts to it.
const GRAMMAR = { js: "typescript", javascript: "typescript", ts: "typescript", bash: "shellscript", sh: "shellscript", py: "python", yml: "yaml" };
export const grammarOf = lang => (lang ? GRAMMAR[lang.toLowerCase()] ?? lang.toLowerCase() : undefined);

const OPTIONS = { themeType: "system", theme: { dark: "pierre-dark", light: "pierre-light" }, lineDiffType: "word", diffIndicators: "bars" };

export function PierreDiff({ oldFile, newFile, patch, split = true, wrap = false, maxLines }) {
  const host = useRef();
  const [error, setError] = useState();
  useEffect(() => {
    let views = [];
    let gone = false;
    load().then(({ FileDiff, parsePatchFiles }) => {
      if (gone) return;
      const options = { ...OPTIONS, diffStyle: split ? "split" : "unified", overflow: wrap ? "wrap" : "scroll", disableFileHeader: !patch };
      const inputs = patch ? parsePatchFiles(patch).flatMap(p => p.files).map(fileDiff => ({ fileDiff })) : [{ oldFile, newFile }];
      views = inputs.map(input => {
        const view = new FileDiff(options);
        view.render({ ...input, containerWrapper: host.current });
        return view;
      });
    }).catch(err => setError(err.message));
    return () => {
      gone = true;
      views.forEach(v => v.cleanUp());
      if (host.current) host.current.textContent = "";
    };
  }, [oldFile?.contents, newFile?.contents, oldFile?.lang, patch, split, wrap]);
  if (error) return html`<div class="artifact-error">Diff view failed: ${error}</div>`;
  return html`<div class="pierre" ref=${host} style=${maxLines ? `--max-lines: ${maxLines}` : undefined}></div>`;
}

// ViewToggle is a pair of buttons in a renderer's own bar: Unified | Split, or Panes | Diff.
export function ViewToggle({ options, value, set }) {
  return html`<span class="view-toggle" role="group">
    ${options.map(([v, label]) => html`<button key=${v} class=${value === v ? "on" : ""} aria-pressed=${value === v} onClick=${() => set(v)}>${label}</button>`)}
  </span>`;
}
