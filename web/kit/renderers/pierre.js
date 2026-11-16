// PierreDiff draws a split or unified diff with @pierre/diffs (web/vendor/diffs.esm.js,
// loaded on first use). It takes two files (oldFile, newFile) or a patch, and draws them in
// the colors of the code panes; r-pierre.css sets the rest of the chrome.
import { html, useEffect, useRef, useState } from "../html.js";

// A TextMate theme whose colors are the page's own CSS variables (r-code.css, tokens.css).
// Variables inherit into the diff's shadow root, so one theme follows light and dark mode.
const THEME = {
  name: "evalkit",
  type: "dark",
  colors: {
    "editor.background": "var(--panel)",
    "editor.foreground": "var(--text)",
    "gitDecoration.addedResourceForeground": "var(--positive-text)",
    "gitDecoration.deletedResourceForeground": "var(--negative-text)",
    "gitDecoration.modifiedResourceForeground": "var(--accent)",
  },
  tokenColors: [
    { scope: ["comment", "punctuation.definition.comment"], settings: { foreground: "var(--code-comment)" } },
    { scope: ["string", "punctuation.definition.string", "constant.character", "constant.other.placeholder"], settings: { foreground: "var(--code-string)" } },
    { scope: ["keyword", "storage", "constant.language", "variable.language"], settings: { foreground: "var(--code-keyword)", fontStyle: "bold" } },
    { scope: ["constant.numeric", "constant.other.rune", "keyword.other.unit"], settings: { foreground: "var(--code-number)", fontStyle: "" } },
    { scope: ["keyword.operator", "punctuation"], settings: { foreground: "var(--code-punct)", fontStyle: "" } },
    { scope: ["entity.name.function", "support.function"], settings: { foreground: "var(--code-function)" } },
    // Go scopes its builtin types (int, string, error) as storage.type; draw them as types.
    { scope: ["entity.name.type", "entity.name.class", "entity.other.inherited-class", "support.type", "support.class", "source.go storage.type"], settings: { foreground: "var(--code-type)", fontStyle: "" } },
  ],
};

let lib;
const load = () => (lib ??= import("/vendor/diffs.esm.js").then(m => {
  m.registerCustomTheme(THEME.name, () => Promise.resolve(THEME));
  return m;
}));

// The bundle carries one grammar per family; map the ids the code renderer accepts to it.
const GRAMMAR = { js: "typescript", javascript: "typescript", ts: "typescript", bash: "shellscript", sh: "shellscript", py: "python", yml: "yaml" };
const grammarOf = lang => (lang ? GRAMMAR[lang.toLowerCase()] ?? lang.toLowerCase() : undefined);

// The same theme on both sides keeps Pierre on the page's color scheme instead of forcing one.
// word-alt joins neighbouring changed words into one highlight, and +/- signs match the unified view.
const OPTIONS = { themeType: "system", theme: { dark: THEME.name, light: THEME.name }, lineDiffType: "word-alt", diffIndicators: "classic" };

export function PierreDiff({ oldFile, newFile, patch, split = true, wrap = false, maxLines }) {
  const host = useRef();
  const [error, setError] = useState();
  useEffect(() => {
    let views = [];
    let gone = false;
    load().then(({ FileDiff, getFiletypeFromFileName, parsePatchFiles, languageNames }) => {
      if (gone) return;
      // A language the bundle carries no grammar for draws as plain text.
      const known = new Set(languageNames);
      const langOf = id => (known.has(grammarOf(id)) ? grammarOf(id) : "text");
      // Two whole files are short snippets the reviewer reads in full, so nothing folds away.
      const options = { ...OPTIONS, diffStyle: split ? "split" : "unified", overflow: wrap ? "wrap" : "scroll", disableFileHeader: !patch, expandUnchanged: !patch };
      const inputs = patch
        ? parsePatchFiles(patch).flatMap(p => p.files).map(fd => ({ fileDiff: { ...fd, lang: langOf(fd.lang ?? getFiletypeFromFileName(fd.name)) } }))
        : [{ oldFile: { ...oldFile, lang: langOf(oldFile.lang) }, newFile: { ...newFile, lang: langOf(newFile.lang) } }];
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
