import assert from "node:assert/strict";
import { test } from "node:test";

import { hydrateReady } from "./bootstrap";

test("hydrateReady is a promise that installRuntimeClient can await", async () => {
  assert.equal(typeof hydrateReady.then, "function");
  await hydrateReady;
});
