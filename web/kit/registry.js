// The registry maps a schema kind to the component that draws it. Built-ins register
// in builtins.js; a suite's own kinds load from its `renderers` map at startup.
const renderers = new Map();
const controls = new Map();

export const registerRenderer = (kind, component) => renderers.set(kind, component);
export const registerControl = (kind, component) => controls.set(kind, component);
export const rendererFor = kind => renderers.get(kind);
export const controlFor = kind => controls.get(kind);

// loadSuiteRenderers imports each module listed under the suite's `renderers`.
export async function loadSuiteRenderers(map = {}) {
  const failed = [];
  for (const [kind, file] of Object.entries(map)) {
    try {
      registerRenderer(kind, (await import(`/files/${file}`)).default);
    } catch (err) {
      failed.push(`${kind}: ${file}: ${err.message}`);
    }
  }
  return failed;
}
