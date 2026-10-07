import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const css = readFileSync(new URL("../../../index.css", import.meta.url), "utf8");

function rule(selector: string): string {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return css.match(new RegExp(`${escaped}\\s*\\{([^}]*)\\}`))?.[1] ?? "";
}

test("app drag regions support both Wails and Electron", () => {
  assert.match(rule(".app-drag"), /--wails-non-client-region:\s*caption/);
  assert.match(rule(".app-drag"), /--wails-draggable:\s*drag/);
  assert.match(rule(".app-drag"), /-webkit-app-region:\s*drag/);
});

test("interactive titlebar controls cancel Wails and Electron drag", () => {
  assert.match(rule(".app-no-drag"), /--wails-non-client-region:\s*none/);
  assert.match(rule(".app-no-drag"), /--wails-draggable:\s*no-drag/);
  assert.match(rule(".app-no-drag"), /-webkit-app-region:\s*no-drag/);
});
