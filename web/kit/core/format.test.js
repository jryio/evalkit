import { test } from "node:test";
import assert from "node:assert/strict";
import { signed, withUnit } from "./format.js";

test("withUnit puts a currency after the sign and other units after the number", () => {
  assert.deepEqual([0.22, -0.1, 0.1, 0].map(v => withUnit(signed(v, 2), "$")), ["+$0.22", "−$0.10", "+$0.10", "±$0"]);
  assert.deepEqual([-0.004, 0.0004, 0.000001].map(v => signed(v, 2)), ["−0.004", "+0.0004", "+0.00000"]);
  assert.equal(withUnit("0.66", "$"), "$0.66");
  assert.equal(withUnit("−3", "%"), "−3%");
  assert.equal(withUnit("+1.5", "s"), "+1.5 s");
  assert.equal(withUnit("7", undefined), "7");
});
