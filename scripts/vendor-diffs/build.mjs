// Bundles entry.js into one ES module. Modules evalkit never runs are replaced by stubs:
// the full Shiki language and theme catalogs (entry.js registers what it needs), the
// WebAssembly regex engine (the JavaScript engine is the default) and @pierre/highlights,
// the package's alternative highlighter.
import { readFileSync, readdirSync, writeFileSync } from "node:fs";
import { build } from "esbuild";

const unused = (what) => `throw new Error("${what} is not bundled in evalkit")`;
const stubs = {
  "shiki/langs": "export const bundledLanguages = {};",
  "shiki/wasm": "export default undefined;",
  "shiki/engine/oniguruma": `export function createOnigurumaEngine() { ${unused("the WebAssembly regex engine")} }`,
  "@pierre/highlights": `export class LiveTokenizer { constructor() { ${unused("@pierre/highlights")} } }
export function codeToTokens() { ${unused("@pierre/highlights")} }
export const isSupportedLanguage = () => false;`,
  "@pierre/highlights/themes/loader": "export const themes = {};",
  "@pierre/theming/themes": "export const themes = { getTheme: () => undefined };",
};

const result = await build({
  entryPoints: [new URL("./entry.js", import.meta.url).pathname],
  outfile: process.argv[2],
  metafile: true,
  bundle: true,
  format: "esm",
  target: "es2022",
  minify: true,
  legalComments: "none",
  logLevel: "warning",
  plugins: [{
    name: "stubs",
    setup(b) {
      b.onResolve({ filter: /.*/ }, (a) => (a.path in stubs ? { path: a.path, namespace: "stub" } : undefined));
      b.onLoad({ filter: /.*/, namespace: "stub" }, (a) => ({ contents: stubs[a.path], loader: "js" }));
    },
  }],
});

// The vendored module must load nothing else.
const leftover = Object.values(result.metafile.outputs).flatMap((o) => o.imports);
if (leftover.length) throw new Error(`bundle still imports: ${leftover.map((i) => i.path).join(", ")}`);

// The license of every package that ended up in the bundle, for process.argv[3].
const dirs = new Set(Object.keys(result.metafile.inputs)
  .map((p) => p.match(/^(.*node_modules\/(?:@[^/]+\/)?[^/]+)\//)?.[1])
  .filter(Boolean));
const sections = [...dirs].sort().map((dir) => {
  const pkg = JSON.parse(readFileSync(`${dir}/package.json`, "utf8"));
  const file = readdirSync(dir).find((f) => /^licen[cs]e/i.test(f));
  const text = file ? readFileSync(`${dir}/${file}`, "utf8").trim() : `License: ${pkg.license}`;
  return `--- ${pkg.name} ${pkg.version} ---\n\n${text}\n`;
});
writeFileSync(process.argv[3], sections.join("\n"));
