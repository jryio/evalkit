const test = require("node:test");
const assert = require("node:assert/strict");
const f = require("../lib/format.js");

test("clock", () => {
  assert.equal(f.clock(7, 5), "07:05"); assert.equal(f.clock(12, 30), "12:30"); assert.equal(f.clock(0, 0), "00:00");
});
