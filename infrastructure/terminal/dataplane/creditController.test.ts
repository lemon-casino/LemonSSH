import assert from "node:assert/strict";
import { test } from "node:test";
import { CreditController } from "./creditController";

test("credit controller honours the initial window and applied sequence", () => {
  const controller = new CreditController();
  const open = controller.openWindow();
  assert.equal(open.credit, 1024 * 1024);
  assert.equal(open.appliedSequence, 0);
  const credit = controller.onApplied(512);
  assert.equal(credit, 512);
  assert.equal(controller.appliedSequence, 512);
});

test("credit controller rejects a second window open", () => {
  const controller = new CreditController();
  controller.openWindow();
  assert.throws(() => controller.openWindow());
});
