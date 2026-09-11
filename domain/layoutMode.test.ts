import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

import { DEFAULT_LAYOUT_MODE, parseLayoutMode } from "./layoutMode";

test("parseLayoutMode accepts the two known modes", () => {
  assert.equal(parseLayoutMode("classic"), "classic");
  assert.equal(parseLayoutMode("workbench"), "workbench");
});

test("parseLayoutMode falls back to null for unknown or non-string input", () => {
  assert.equal(parseLayoutMode("zen"), null);
  assert.equal(parseLayoutMode(""), null);
  assert.equal(parseLayoutMode(42), null);
  assert.equal(parseLayoutMode(null), null);
  assert.equal(parseLayoutMode(undefined), null);
  assert.equal(parseLayoutMode({}), null);
});

test("default layout mode is classic", () => {
  assert.equal(DEFAULT_LAYOUT_MODE, "classic");
});

test("STORAGE_KEY_LAYOUT_MODE is registered in both sync channels", async () => {
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
  const ipcSrc = await readFile(path.join(root, "application/state/settingsIpcSync.ts"), "utf8");
  const storageSrc = await readFile(path.join(root, "application/state/settingsStorageSync.ts"), "utf8");
  assert.ok(ipcSrc.includes("STORAGE_KEY_LAYOUT_MODE"), "settingsIpcSync.ts must carry STORAGE_KEY_LAYOUT_MODE");
  assert.ok(storageSrc.includes("STORAGE_KEY_LAYOUT_MODE"), "settingsStorageSync.ts must carry STORAGE_KEY_LAYOUT_MODE");
  assert.ok(ipcSrc.includes("setLayoutMode"), "settingsIpcSync.ts must call setLayoutMode");
  assert.ok(storageSrc.includes("setLayoutMode"), "settingsStorageSync.ts must call setLayoutMode");
});
