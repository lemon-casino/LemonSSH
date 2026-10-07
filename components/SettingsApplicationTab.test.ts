import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { buildIssueUrl } from "./SettingsApplicationTab.tsx";

test("application settings only expose update check and feedback to LemonSSH", () => {
  const source = readFileSync(join(process.cwd(), "components/SettingsApplicationTab.tsx"), "utf8");
  assert.match(source, /application-check-updates/);
  assert.match(source, /application-report-problem/);
  assert.doesNotMatch(source, /application-community/);
  assert.doesNotMatch(source, /application-github/);
  assert.doesNotMatch(source, /application-whats-new/);
  assert.match(source, /https:\/\/github\.com\/lemon-casino\/LemonSSH/);
  const issueUrl = buildIssueUrl({ name: "LemonSSH", version: "1.0.0", platform: "win32" });
  assert.match(issueUrl, /^https:\/\/github\.com\/lemon-casino\/LemonSSH\/issues\/new\?/);
});
