const test = require("node:test");
const assert = require("node:assert/strict");
const f = require("../lib/format.js");

test("truncate", () => {
  assert.equal(f.truncate("abcdefgh", 5), "abcd…"); assert.equal(f.truncate("abc", 5), "abc"); assert.equal(f.truncate("abcde", 5), "abcde");
});
