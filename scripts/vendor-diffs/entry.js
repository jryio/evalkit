// The parts of @pierre/diffs evalkit uses: the split and unified diff view and the languages
// evalkit highlights. The page registers its own theme (web/kit/renderers/pierre.js), so no
// Pierre theme ships. Everything else is stubbed out in build.mjs. Bundled by
// scripts/vendor-diffs.sh into web/vendor/diffs.esm.js.
// The TypeScript grammar also serves JavaScript; evalkit maps the language ids.
import { FileDiff, getFiletypeFromFileName, parsePatchFiles, registerCustomLanguage, registerCustomTheme } from "@pierre/diffs";

const languages = {
  go: () => import("@shikijs/langs/go"),
  python: () => import("@shikijs/langs/python"),
  typescript: () => import("@shikijs/langs/typescript"),
  json: () => import("@shikijs/langs/json"),
  yaml: () => import("@shikijs/langs/yaml"),
  shellscript: () => import("@shikijs/langs/shellscript"),
  sql: () => import("@shikijs/langs/sql"),
  diff: () => import("@shikijs/langs/diff"),
};
for (const [name, load] of Object.entries(languages)) registerCustomLanguage(name, load);

// The page draws any other language as plain text instead of failing to load it.
export const languageNames = Object.keys(languages);
export { FileDiff, getFiletypeFromFileName, parsePatchFiles, registerCustomTheme };
