import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { getReleaseUrl } from "./updateService.ts";

test("update checks and release links point at LemonSSH", () => {
  const source = readFileSync(join(process.cwd(), "infrastructure/services/updateService.ts"), "utf8");
  assert.match(source, /api\.github\.com\/repos\/lemon-casino\/LemonSSH\/releases\/latest/);
  assert.match(source, /github\.com\/lemon-casino\/LemonSSH\/releases/);
  assert.doesNotMatch(source, /binaricat\/Netcatty/);
  assert.equal(getReleaseUrl(), "https://github.com/lemon-casino/LemonSSH/releases");
  assert.equal(getReleaseUrl("1.2.3"), "https://github.com/lemon-casino/LemonSSH/releases/tag/v1.2.3");
});
