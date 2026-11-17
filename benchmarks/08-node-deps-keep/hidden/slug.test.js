const test = require("node:test");
const assert = require("node:assert/strict");
const f = require("../lib/format.js");

test("slug", () => {
  assert.equal(f.slug("Hello, World!"), "hello-world"); assert.equal(f.slug("  a  b  "), "a-b"); assert.equal(f.slug("x"), "x");
});
