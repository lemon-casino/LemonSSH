const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const workflowsDir = path.join(__dirname, "..", ".github", "workflows");
const readWorkflow = (name) => fs.readFileSync(path.join(workflowsDir, name), "utf8");

const testWorkflow = readWorkflow("test.yml");
const buildWorkflow = readWorkflow("build.yml");
const cursorWorkflow = readWorkflow("cursor-automation.yml");
const etWorkflow = readWorkflow("build-et-binaries.yml");
const appBuilderPatch = fs.readFileSync(
  path.join(__dirname, "..", "patches", "app-builder-lib+26.15.2.patch"),
  "utf8",
);
const windowsEtBuild = fs.readFileSync(
  path.join(__dirname, "build-et", "build-windows.ps1"),
  "utf8",
);
const homebrewBump = fs.readFileSync(
  path.join(__dirname, "..", ".github", "scripts", "bump-homebrew-cask.sh"),
  "utf8",
);

const pullRequestPaths = buildWorkflow
  .match(/pull_request:\s*\n\s*paths:\s*\n((?:\s+- "[^"]+"\s*\n)+)/)?.[1]
  ?.match(/^\s+- "([^"]+)"$/gm)
  ?.map((line) => line.match(/^\s+- "([^"]+)"$/)?.[1])
  .filter(Boolean);

// Portable glob matcher for Node >=22 (path.matchesGlob only landed in 22.5).
const matchesGlob = (filePath, pattern) => {
  let regex = "^";
  for (let i = 0; i < pattern.length; ) {
    if (pattern[i] === "*" && pattern[i + 1] === "*") {
      if (pattern[i + 2] === "/") {
        regex += "(?:.*/)?";
        i += 3;
      } else {
        regex += ".*";
        i += 2;
      }
      continue;
    }
    if (pattern[i] === "*") {
      regex += "[^/]*";
      i += 1;
      continue;
    }
    if (pattern[i] === "?") {
      regex += "[^/]";
      i += 1;
      continue;
    }
    regex += pattern[i].replace(/[|\\{}()[\]^$+?.]/g, "\\$&");
    i += 1;
  }
  regex += "$";
  return new RegExp(regex).test(filePath);
};

const triggersPackageValidation = (filePath) => {
  assert.ok(pullRequestPaths, "package workflow pull_request paths must be readable");
  return pullRequestPaths.reduce((included, pattern) => {
    const excluded = pattern.startsWith("!");
    const glob = excluded ? pattern.slice(1) : pattern;
    return matchesGlob(filePath, glob) ? !excluded : included;
  }, false);
};

test("PR validation runs once per commit and includes a production build", () => {
  assert.match(testWorkflow, /push:\s*\n\s*branches:\s*\n\s*- main/);
  assert.doesNotMatch(testWorkflow, /branches:\s*\n\s*- "\*\*"/);
  assert.match(testWorkflow, /name: lint-and-test\s*\n\s*runs-on: ubuntu-latest\s*\n\s*timeout-minutes: 20/);
  assert.match(testWorkflow, /sudo apt-get install -y fish xvfb libgtk-3-dev libwebkit2gtk-4\.1-dev libgtk-4-dev libwebkitgtk-6\.0-dev/);
  assert.match(
    testWorkflow,
    /- name: Test Go owners\s*\n\s*run: go test \.\/internal\/\.\.\. \.\/cmd\/\.\.\./,
  );
  assert.match(
    buildWorkflow,
    /- name: Test macOS Option column selection\s*\n\s*if: matrix\.name == 'macos'\s*\n\s*run: npm run test:xterm-macos-selection/,
  );
  assert.match(testWorkflow, /- name: Build Wails frontend\s*\n\s*run: npm run build/);
  assert.match(testWorkflow, /node scripts\/wails-prepare-frontend\.mjs/);
  assert.doesNotMatch(testWorkflow, /check:migration-electron-baseline/);
  assert.doesNotMatch(testWorkflow, /xterm-keyword-highlight-performance\.live\.test\.cjs/);
  assert.match(
    testWorkflow,
    /- name: Verify Wails migration documentation\s*\n\s*run: npm run check:migration-docs/,
  );
  assert.match(testWorkflow, /uses: actions\/setup-go@v6/);
  assert.match(
    testWorkflow,
    /- name: Verify Wails shell probe\s*\n\s*run: npm run check:wails-shell-probe/,
  );
  assert.match(
    testWorkflow,
    /cache-dependency-path: \|\s*\n\s*experiments\/wails-shell-probe\/go\.sum\s*\n\s*experiments\/terminal-data-plane\/go\.sum/,
  );
  assert.match(
    testWorkflow,
    /- name: Verify terminal data-plane probe\s*\n\s*run: npm run check:terminal-data-plane-probe/,
  );
  assert.doesNotMatch(testWorkflow, /\n  mosh-windows-conpty:/);
});

test("Wails package workflow uploads unsigned binaries only", () => {
  const workflow = readWorkflow("wails-package.yml");
  assert.match(workflow, /node scripts\/package-wails\.mjs/);
  assert.match(workflow, /sign-wails-probe\.mjs/);
  assert.match(workflow, /name: lemonssh-\$\{\{ matrix\.os \}\}/);
  assert.match(workflow, /lemonssh-windows-latest/);
  assert.match(workflow, /lemonssh-macos-latest/);
  assert.match(workflow, /lemonssh-ubuntu-latest/);
  assert.doesNotMatch(workflow, /lemonssh-unsigned-/);
  assert.doesNotMatch(workflow, /signtool/);
  assert.doesNotMatch(workflow, /notarytool|altool|APPLE_ID|WINDOWS_CERT/);
});

test("Wails package workflow publishes unsigned GitHub Releases on v tags", () => {
  const workflow = readWorkflow("wails-package.yml");
  assert.match(workflow, /tags:\s*\n\s*- v\*/);
  assert.match(workflow, /softprops\/action-gh-release@/);
  assert.match(workflow, /startsWith\(github\.ref, 'refs\/tags\/v'\)/);
  assert.match(workflow, /contents: write/);
  assert.doesNotMatch(workflow, /signtool|notarytool|APPLE_ID|WINDOWS_CERT/);
});

test("package release concurrency is isolated per tag", () => {
  assert.match(buildWorkflow, /format\('release-\{0\}', github\.ref\)/);
  assert.doesNotMatch(buildWorkflow, /&& 'release' \|\| github\.ref/);
});

test("manual package validations do not share push concurrency", () => {
  assert.match(
    buildWorkflow,
    /github\.event_name == 'workflow_dispatch' && format\('manual-\{0\}', github\.run_id\)/,
  );
  assert.ok(
    buildWorkflow.indexOf("format('release-{0}', github.ref)") <
      buildWorkflow.indexOf("format('manual-{0}', github.run_id)"),
    "publishing a tag manually must still share that tag's release group",
  );
});

test("package validation avoids duplicate branch runs and scopes PR builds", () => {
  // LemonSSH: the legacy Electron packaging pipeline is manual-dispatch only;
  // the migration builds its own packaging in P6-02.
  assert.doesNotMatch(buildWorkflow, /^  push:/m);
  assert.doesNotMatch(buildWorkflow, /^  pull_request:/m);
  assert.match(buildWorkflow, /workflow_dispatch:/);
  assert.doesNotMatch(buildWorkflow, /\n  dedupe:/);
  assert.doesNotMatch(buildWorkflow, /\n  dedupe-result:/);
  // Path-scoped PR filters were removed with the dispatch-only trigger; the
  // triggersPackageValidation helper below no longer applies.
});

test("Homebrew tap updates retry push races without downgrading newer releases", () => {
  assert.match(homebrewBump, /MAX_PUSH_ATTEMPTS/);
  assert.match(homebrewBump, /version_is_newer/);
  assert.match(homebrewBump, /git fetch --depth=1 origin main/);
  assert.match(homebrewBump, /git switch -C main origin\/main/);
  assert.match(homebrewBump, /for \(\(attempt=1; attempt<=MAX_PUSH_ATTEMPTS; attempt\+\+\)\)/);
  assert.match(homebrewBump, /if version_is_newer "\$current_version" "\$VERSION"/);
  assert.match(homebrewBump, /if push_output="\$\(git push origin HEAD:main 2>&1\)"/);
  assert.match(homebrewBump, /grep -Eqi 'non-fast-forward\|fetch first' <<<"\$push_output"/);
  assert.doesNotMatch(homebrewBump, /2> >\(tee/);
  assert.match(homebrewBump, /Tap already has newer version/);
  assert.match(homebrewBump, /Push raced with another release/);
});

test("reused automation PRs still receive labels and one source-issue backlink", () => {
  const openPr = cursorWorkflow.match(
    /\n      - name: Open draft PR[\s\S]*?(?=\n      - name: Request Codex review on implement PR)/,
  );
  assert.ok(openPr, "open-PR step must exist before Codex review request");
  assert.match(openPr[0], /github\.rest\.issues\.addLabels/);
  assert.match(openPr[0], /github\.paginate\(github\.rest\.issues\.listComments/);
  assert.match(openPr[0], /auto\.hasAutomationPullRequestBacklink/);
  assert.doesNotMatch(openPr[0], /if \(created\)/);
});

test("reused implementation PRs do not duplicate Codex requests for the same head", () => {
  const requestCodex = cursorWorkflow.match(
    /\n      - name: Request Codex review on implement PR[\s\S]*?(?=\n  codex_loop:)/,
  );
  assert.ok(requestCodex, "implement Codex request step must exist before codex_loop");
  assert.match(requestCodex[0], /github\.paginate\(github\.rest\.issues\.listComments/);
  assert.match(requestCodex[0], /github\.rest\.pulls\.get/);
  assert.match(requestCodex[0], /auto\.shouldSkipExternalCodexRerequest/);
  assert.match(requestCodex[0], /headSha/);
  assert.match(requestCodex[0], /OWN_ACTORS/);
  assert.doesNotMatch(requestCodex[0], /process\.env\.HEAD_SHA/);
});

test("clean Codex handoff updates labels without GraphQL-only organization scopes", () => {
  const markReady = cursorWorkflow.match(/\n      - name: Mark PR ready after clean Codex[\s\S]*?(?=\n      - name: Give up after max rounds)/);
  assert.ok(markReady, "mark-ready step must exist before give-up step");
  assert.match(markReady[0], /gh api/);
  assert.match(markReady[0], /issues\/\$\{PULL_NUMBER\}\/labels/);
  assert.match(markReady[0], /automation%3Acodex-loop/);
  assert.match(markReady[0], /ready-for-human/);
  assert.match(markReady[0], /labels\[\]=automation:codex-clean/);
  assert.match(markReady[0], /labels\[\]=automation:bot-pr/);
  assert.doesNotMatch(markReady[0], /nextCodexTerminalLabels/);
  assert.doesNotMatch(markReady[0], /"PUT"/);
  assert.doesNotMatch(markReady[0], /gh pr edit/);
});

test("permission handoffs use the established ready-for-human label", () => {
  assert.doesNotMatch(cursorWorkflow, /automation:needs-human/);
  assert.match(cursorWorkflow, /labels: \['ready-for-human'\]/);
  assert.match(cursorWorkflow, /-f 'labels\[\]=ready-for-human'/);
});

test("ET binary validation runs once and retries transient container pulls", () => {
  assert.match(etWorkflow, /push:\s*\n\s*branches:\s*\n\s*- main/);
  assert.doesNotMatch(etWorkflow, /branches:\s*\n\s*- "\*\*"/);
  assert.match(etWorkflow, /Pull build container with retry/g);
  assert.match(etWorkflow, /docker pull/);
  assert.match(etWorkflow, /--pull=never/);
  assert.match(etWorkflow, /Restore vcpkg download cache/g);
  assert.match(etWorkflow, /VCPKG_DOWNLOADS/g);
  assert.match(windowsEtBuild, /Invoke-WithRetry/);
});

test("GitHub-owned actions use current Node 24 releases", () => {
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
      const uses = [...source.matchAll(new RegExp(`${action.replace("/", "\\/")}@(v\\d+)`, "g"))];
      for (const match of uses) {
        assert.equal(match[1], major, `${name} must use ${action}@${major}`);
      }
    }
  }
});

test("electron-builder retries Fetch API server errors", () => {
  assert.match(appBuilderPatch, /e\?\.response\?\.status/);
  assert.match(appBuilderPatch, /responseStatus >= 500/);
});
