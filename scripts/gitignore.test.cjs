const assert = require("node:assert/strict");
const { spawnSync } = require("node:child_process");
const path = require("node:path");
const test = require("node:test");

const root = path.resolve(__dirname, "..");

test("root TypeScript tests are visible to Git while scratch files remain ignored", () => {
  for (const file of ["regression.test.ts", "regression.test.tsx", "domain/regression.test.ts"]) {
    const result = spawnSync("git", ["check-ignore", "--no-index", file], { cwd: root, encoding: "utf8" });
    assert.equal(result.status, 1, `${file} must be trackable: ${result.stdout}${result.stderr}`);
  }
  for (const file of ["scratch.ts", "scratch.txt", ".env.test", "node_modules/regression.test.ts"]) {
    const result = spawnSync("git", ["check-ignore", "--no-index", file], { cwd: root, encoding: "utf8" });
    assert.equal(result.status, 0, `${file} must remain ignored: ${result.stderr}`);
  }
});
