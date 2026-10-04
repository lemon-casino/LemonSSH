import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { getReleaseUrl, isDevVersion } from "./updateService.ts";

test("update checks and release links point at LemonSSH", () => {
  const source = readFileSync(join(process.cwd(), "infrastructure/services/updateService.ts"), "utf8");
  assert.match(source, /api\.github\.com\/repos\/lemon-casino\/LemonSSH\/releases\/latest/);
  assert.match(source, /github\.com\/lemon-casino\/LemonSSH\/releases/);
  // The update source must never point back at a pre-rename identity: the old
  // binaricat org or any Netcatty repo slug (binaricat/Netcatty,
  // lemon-casino/Netcatty). The exact lemon-casino/LemonSSH targets above are
  // asserted positively; these guards only cover the stale names.
  assert.doesNotMatch(source, /binaricat\//);
  assert.doesNotMatch(source, /netcatty/i);
  assert.equal(getReleaseUrl(), "https://github.com/lemon-casino/LemonSSH/releases");
  assert.equal(getReleaseUrl("1.2.3"), "https://github.com/lemon-casino/LemonSSH/releases/tag/v1.2.3");
});

test("isDevVersion skips zero and stamped dev builds", () => {
  for (const version of ["", "  ", "0.0.0", "0.0.0-dev", "0.0.0-wails-skeleton", "v0", "0"]) {
    assert.equal(isDevVersion(version), true, `isDevVersion(${JSON.stringify(version)}) should be true`);
  }
  for (const version of ["0.0.1", "1.2.3", "0.1.0", "v1.0.0"]) {
    assert.equal(isDevVersion(version), false, `isDevVersion(${JSON.stringify(version)}) should be false`);
  }
});
