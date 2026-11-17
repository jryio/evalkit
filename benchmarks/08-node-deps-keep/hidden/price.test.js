const test = require("node:test");
const assert = require("node:assert/strict");
const f = require("../lib/format.js");

test("price", () => {
  assert.equal(f.price(1250), "$12.50"); assert.equal(f.price(500), "$5.00"); assert.equal(f.price(5), "$0.05");
});
