// The server API. Every call throws on a non-2xx response.
async function call(path, body) {
  const init = body === undefined ? {} : { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) };
  const res = await fetch(path, init);
  if (!res.ok) throw Object.assign(new Error(`${path}: ${res.status} ${await res.text()}`), { status: res.status });
  return res.status === 204 ? null : res.json();
}

export const api = {
  suite: () => call("/api/suite"),
  version: () => call("/api/version"),
  state: () => call("/api/state"),
  saveState: state => call("/api/state", state),
  submit: (state, snapshot) => call("/api/submit", { state, snapshot }),
};
