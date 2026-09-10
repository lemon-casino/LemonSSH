import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

test("wails:build uses the Windows GUI subsystem", () => {
  const pkg = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8"));
  assert.match(pkg.scripts["wails:build"], /-ldflags="-H windowsgui"/);
  assert.match(pkg.scripts["wails:build"], /-o bin\/LemonSSH\.exe/);
});
