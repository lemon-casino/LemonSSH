import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

test("wails:build uses the Windows GUI subsystem", () => {
  const pkg = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8"));
  assert.equal(pkg.scripts["wails:build"], "node scripts/wails-build.mjs");
  const buildScript = readFileSync(new URL("./wails-build.mjs", import.meta.url), "utf8");
  assert.match(buildScript, /-H windowsgui/);
  assert.match(buildScript, /-o bin\/LemonSSH\.exe/);
});
