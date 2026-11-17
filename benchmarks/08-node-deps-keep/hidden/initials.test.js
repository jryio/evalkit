const test = require("node:test");
const assert = require("node:assert/strict");
const f = require("../lib/format.js");

test("initials", () => {
  assert.equal(f.initials("ada lovelace"), "AL"); assert.equal(f.initials("ada  lovelace"), "AL"); assert.equal(f.initials(" grace hopper "), "GH");
});
