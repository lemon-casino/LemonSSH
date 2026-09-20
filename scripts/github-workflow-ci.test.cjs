const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const workflowsDir = path.join(__dirname, "..", ".github", "workflows");
const readWorkflow = (name) => fs.readFileSync(path.join(workflowsDir, name), "utf8");

test("CI validates the Wails application", () => {
  const workflow = readWorkflow("test.yml");
  assert.match(workflow, /go test \.\/internal\/\.\.\. \.\/cmd\/\.\.\./);
  assert.match(workflow, /npm test/);
  assert.match(workflow, /npm run build/);
  assert.match(workflow, /go build -o \/tmp\/lemonssh-ci \.\/cmd\/netcatty/);
  assert.match(workflow, /npm run generate:capability-tools/);
  assert.doesNotMatch(workflow, /electron|migration-docs|secret-migration-probe/i);
});

test("Wails packaging publishes current-source artifacts without a signing gate", () => {
  const workflow = readWorkflow("wails-package.yml");
  assert.match(workflow, /node scripts\/package-wails\.mjs/);
  assert.match(workflow, /softprops\/action-gh-release@v2/);
  assert.match(workflow, /Code signing is\s*\n?# optional distribution metadata|Code signing is optional distribution metadata/i);
  assert.doesNotMatch(workflow, /signtool|notarytool|sign-wails-probe|self-sign/i);
});

test("GitHub-owned actions use the repository's current major versions", () => {
  const workflows = fs.readdirSync(workflowsDir)
    .filter((name) => name.endsWith(".yml"))
    .map((name) => [name, readWorkflow(name)]);
  const expectedMajors = new Map([
    ["actions/checkout", "v7"],
    ["actions/setup-node", "v7"],
    ["actions/upload-artifact", "v7"],
    ["actions/download-artifact", "v8"],
    ["actions/github-script", "v9"],
    ["actions/cache", "v6"],
  ]);
  for (const [name, source] of workflows) {
    for (const [action, major] of expectedMajors) {
      for (const match of source.matchAll(new RegExp(`${action.replace("/", "\\/")}@(v\\d+)`, "g"))) {
        assert.equal(match[1], major, `${name} must use ${action}@${major}`);
      }
    }
  }
});
